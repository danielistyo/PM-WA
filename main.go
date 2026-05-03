package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"pm-wa/bot"
	"pm-wa/cmd"
	"pm-wa/db"
	"pm-wa/reply"
	"pm-wa/scheduler"
)

type App struct {
	client       *bot.Client
	db           *db.Database
	cmdHandler   *cmd.Handler
	replyHandler *reply.Handler
	scheduler    *scheduler.Scheduler
}

func main() {
	cfg := DefaultConfig()

	database, err := db.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	client, err := bot.NewClient(cfg.SessionDBPath)
	if err != nil {
		slog.Error("failed to create WA client", "error", err)
		os.Exit(1)
	}

	app := &App{
		client:       client,
		db:           database,
		cmdHandler:   cmd.NewHandler(client, database),
		replyHandler: reply.NewHandler(client, database),
		scheduler:    scheduler.New(client, database, cfg.ScheduleTime),
	}

	client.WA.AddEventHandler(app.eventHandler)

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
	slog.Info("connected to WhatsApp")
	app.syncGroups()
	app.scheduler.Stop()
	app.scheduler.Start()
}

func (app *App) handleJoinedGroup(evt *events.JoinedGroup) {
	app.db.UpsertGroup(evt.JID.String(), evt.GroupName.Name)
	slog.Info("joined group", "group", evt.GroupName.Name)
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
		slog.Error("failed to sync groups", "error", err)
		return
	}
	for _, g := range groups {
		slog.Info("synced group", "name", g.GroupName.Name, "jid", g.JID.String())
		app.db.UpsertGroup(g.JID.String(), g.GroupName.Name)
	}
	slog.Info("synced groups", "count", len(groups))
}
