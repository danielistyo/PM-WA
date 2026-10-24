package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"pm-wa/accounts"
	"pm-wa/bot"
	"pm-wa/cmd"
	"pm-wa/db"
	"pm-wa/reply"
	"pm-wa/scheduler"
	"pm-wa/web"
)

// App is the per-account composition unit: one WhatsApp client, one task
// database, and the handlers/scheduler over them. Accounts share nothing.
type App struct {
	name         string
	client       *bot.Client
	db           *db.Database
	cmdHandler   *cmd.Handler
	replyHandler *reply.Handler
	scheduler    *scheduler.Scheduler
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "pair":
			runPair(os.Args[2:])
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q; usage: pm-wa [pair <account-name>]\n", os.Args[1])
			os.Exit(2)
		}
	}

	cfg := DefaultConfig()

	res, err := accounts.Resolve(os.Getenv("WA_ACCOUNTS"), accounts.StatFSState())
	if err != nil {
		slog.Error("failed to resolve accounts", "error", err)
		os.Exit(1)
	}

	if err := accounts.Apply(res); err != nil {
		slog.Error("legacy data migration failed", "error", err)
		os.Exit(1)
	}
	warnUnmigratedLegacy()

	if len(res.Accounts) == 1 {
		runSingle(res.Accounts[0], cfg)
		return
	}
	runMulti(res.Accounts, cfg)
}

// runSingle boots the default account exactly as the pre-multi-account bot:
// blocking boot-time QR flow, exit on connect failure, web UI at the bare
// root. Behavior parity is a spec requirement for the unset-WA_ACCOUNTS case.
func runSingle(acct accounts.Account, cfg Config) {
	if err := acct.EnsureDir(); err != nil {
		slog.Error("failed to create account data directory", "error", err)
		os.Exit(1)
	}

	database, err := db.New(acct.AppDBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	client, err := bot.NewClient(acct.SessionDBPath)
	if err != nil {
		slog.Error("failed to create WA client", "error", err)
		os.Exit(1)
	}

	app := newApp(acct, client, database, cfg)

	client.WA.AddEventHandler(app.eventHandler)

	if cfg.WebBaseURL != "" {
		webServer := web.NewServer([]web.MountSpec{
			{Name: acct.Name, DB: database, Client: client},
		}, cfg.WebBaseURL)
		go func() {
			slog.Info("starting web server", "addr", cfg.WebListenAddr)
			if err := webServer.Start(cfg.WebListenAddr); err != nil {
				slog.Error("web server stopped", "error", err)
			}
		}()
	}

	if err := client.Connect(); err != nil {
		slog.Error("failed to connect", "error", err)
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	slog.Info("shutting down")
	app.scheduler.Stop()
	client.Disconnect()
}

// runMulti boots every declared account in the same process. A failing or
// unpaired account is logged and skipped; healthy accounts keep serving.
func runMulti(accts []accounts.Account, cfg Config) {
	apps := make([]*App, 0, len(accts))
	for _, acct := range accts {
		app, err := startAccount(acct, cfg)
		if err != nil {
			slog.Error("failed to start account, skipping", "account", acct.Name, "error", err)
			continue
		}
		if app == nil {
			continue
		}
		apps = append(apps, app)
	}

	if len(apps) == 0 {
		slog.Error("no account could start; pair at least one account with 'pm-wa pair <name>'")
		os.Exit(1)
	}

	if cfg.WebBaseURL != "" {
		// One subserver per account, each mounted under its own /a/<name>/
		// prefix with independent auth; a single listener serves them all.
		mounts := make([]web.MountSpec, 0, len(apps))
		for _, app := range apps {
			mounts = append(mounts, web.MountSpec{Name: app.name, DB: app.db, Client: app.client})
		}
		webServer := web.NewServer(mounts, cfg.WebBaseURL)
		go func() {
			slog.Info("starting web server", "addr", cfg.WebListenAddr)
			if err := webServer.Start(cfg.WebListenAddr); err != nil {
				slog.Error("web server stopped", "error", err)
			}
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	slog.Info("shutting down")
	for _, app := range apps {
		app.scheduler.Stop()
		app.client.Disconnect()
	}
}

// startAccount opens one account's isolated stack and connects it in the
// background so a slow or failing connect cannot delay other accounts.
// An unpaired account returns (nil, nil) after being logged.
func startAccount(acct accounts.Account, cfg Config) (*App, error) {
	if err := acct.EnsureDir(); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	database, err := db.New(acct.AppDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	client, err := bot.NewClient(acct.SessionDBPath)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("failed to create WA client: %w", err)
	}

	if client.WA.Store.ID == nil {
		slog.Warn("account is not paired, skipping", "account", acct.Name, "hint", "run: pm-wa pair "+acct.Name)
		database.Close()
		return nil, nil
	}

	app := newApp(acct, client, database, cfg)
	client.WA.AddEventHandler(app.eventHandler)
	go func() {
		if err := client.Connect(); err != nil {
			slog.Error("account connect failed", "account", acct.Name, "error", err)
		}
	}()
	return app, nil
}

func newApp(acct accounts.Account, client *bot.Client, database *db.Database, cfg Config) *App {
	return &App{
		name:         acct.Name,
		client:       client,
		db:           database,
		cmdHandler:   cmd.NewHandler(client, database, webLinkBase(cfg.WebBaseURL, acct.Name)),
		replyHandler: reply.NewHandler(client, database),
		scheduler:    scheduler.New(client, database),
	}
}

// webLinkBase builds the web base for outbound links of one account. In
// multi-account mode every account mounts under /a/<name>/, so links must
// carry the sending account's prefix.
func webLinkBase(baseURL, accountName string) string {
	if baseURL == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/a/" + accountName
}

// warnUnmigratedLegacy surfaces legacy root data files that were skipped
// (data/default already exists), mirroring the "target exists" resolve case.
func warnUnmigratedLegacy() {
	for _, file := range accounts.LegacyFiles() {
		if _, err := os.Stat(file); err == nil {
			slog.Warn("legacy data file left untouched", "file", file, "hint", "data/default already exists")
		}
	}
}

func (app *App) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsGroup {
			app.handleGroupMessage(v)
		} else {
			app.handlePrivateMessage(v)
		}
	case *events.GroupInfo:
		app.handleGroupInfoChange(v)
	case *events.Connected:
		app.handleConnected()
	case *events.JoinedGroup:
		app.handleJoinedGroup(v)
	}
}

func (app *App) handlePrivateMessage(evt *events.Message) {
	text := bot.ExtractText(evt.Message)
	if text == "" {
		return
	}
	app.cmdHandler.HandleCommand(evt.Info.Sender, text)
}

func (app *App) handleGroupMessage(evt *events.Message) {
	quotedID := bot.GetQuotedMessageID(evt)
	if quotedID == "" {
		return
	}
	text := bot.ExtractText(evt.Message)
	if text == "" {
		return
	}
	ctx := context.Background()
	senderJID := evt.Info.Sender
	senderAlt := evt.Info.SenderAlt
	app.replyHandler.HandleGroupReply(ctx, evt.Info.Chat, senderJID, senderAlt, quotedID, text)
}

func (app *App) handleConnected() {
	slog.Info("connected to WhatsApp", "account", app.name)
	app.syncGroups()
	// Restart the scheduler on every reconnect so cron pulses recover
	// automatically after transient drops.
	app.scheduler.Stop()
	app.scheduler.Start()
}

func (app *App) handleJoinedGroup(evt *events.JoinedGroup) {
	app.db.UpsertGroup(evt.JID.String(), evt.GroupName.Name)
	slog.Info("joined group", "account", app.name, "group", evt.GroupName.Name)
}

func (app *App) handleGroupInfoChange(evt *events.GroupInfo) {
	groupJID := evt.JID
	botJID := app.client.BotJID()

	for _, leftJID := range evt.Leave {
		if leftJID.ToNonAD().User == botJID.User {
			app.handleBotKicked(groupJID)
			return
		}
	}

	for _, leftJID := range evt.Leave {
		app.handleMemberLeft(groupJID, leftJID)
	}
}

func (app *App) handleBotKicked(groupJID types.JID) {
	ctx := context.Background()
	lists, _ := app.db.GetActiveListsByGroup(groupJID.String())
	groupName := app.db.GetGroupName(groupJID.String())

	for _, list := range lists {
		app.db.UpdateListStatus(list.ID, "stopped")
		adminJID, err := types.ParseJID(list.AdminJID)
		if err != nil {
			continue
		}
		msg := fmt.Sprintf("Alert: I was kicked from '%s'. All task lists for this group have been permanently stopped.", groupName)
		app.client.SendPM(ctx, adminJID, msg)
	}

	app.db.DeleteGroup(groupJID.String())
}

func (app *App) handleMemberLeft(groupJID types.JID, memberJID types.JID) {
	ctx := context.Background()
	jidStr := memberJID.ToNonAD().String()
	groupName := app.db.GetGroupName(groupJID.String())

	adminLists, _ := app.db.GetActiveListsByGroupAndAdmin(groupJID.String(), jidStr)
	for _, list := range adminLists {
		app.db.UpdateListStatus(list.ID, "stopped")
		msg := fmt.Sprintf("Alert: You left '%s'. All your task lists for this group have been permanently stopped.", groupName)
		app.client.SendPM(ctx, memberJID, msg)
	}

	affectedTasks, _ := app.db.MarkAssigneeLeft(groupJID.String(), jidStr)

	notifiedAdmins := make(map[string]bool)
	for _, task := range affectedTasks {
		list, err := app.db.GetTaskList(task.TaskListID)
		if err != nil || list.Status != "active" {
			continue
		}
		if notifiedAdmins[list.AdminJID] {
			continue
		}
		notifiedAdmins[list.AdminJID] = true
		adminJID, err := types.ParseJID(list.AdminJID)
		if err != nil {
			continue
		}
		msg := fmt.Sprintf("Alert: Assignee %s has left '%s'. Their tasks will show '(1 assignee left)' in reminders. Use 'delete-task' and re-add with a valid assignee to fix.",
			memberJID.ToNonAD().User, groupName)
		app.client.SendPM(ctx, adminJID, msg)
	}
}

func (app *App) syncGroups() {
	ctx := context.Background()
	groups, err := app.client.GetJoinedGroups(ctx)
	if err != nil {
		slog.Error("failed to sync groups", "account", app.name, "error", err)
		return
	}
	for _, g := range groups {
		slog.Info("synced group", "account", app.name, "name", g.GroupName.Name, "jid", g.JID.String())
		app.db.UpsertGroup(g.JID.String(), g.GroupName.Name)
	}
	slog.Info("synced groups", "account", app.name, "count", len(groups))
}
