package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
	"pm-wa/db"
	"pm-wa/format"
)

type Handler struct {
	client     *bot.Client
	db         *db.Database
	webBaseURL string
}

func NewHandler(client *bot.Client, database *db.Database, webBaseURL string) *Handler {
	return &Handler{
		client:     client,
		db:         database,
		webBaseURL: webBaseURL,
	}
}

func (h *Handler) HandleCommand(senderJID types.JID, text string) {
	parsed, err := ParseCommand(text)
	if err != nil {
		return
	}

	ctx := context.Background()

	switch parsed.Command {
	case "add-list":
		h.addList(ctx, senderJID, parsed.Fields)
	case "add-task":
		h.addTask(ctx, senderJID, parsed.Fields)
	case "publish-list":
		h.publishList(ctx, senderJID, parsed.Fields)
	case "stop-list":
		h.stopList(ctx, senderJID, parsed.Fields)
	case "delete-task":
		h.deleteTask(ctx, senderJID, parsed.Fields)
	case "delete-list":
		h.deleteList(ctx, senderJID, parsed.Fields)
	case "list-lists":
		h.listLists(ctx, senderJID, parsed.Fields)
	case "list-tasks":
		h.listTasks(ctx, senderJID, parsed.Fields)
	case "tasklist-web":
		h.tasklistWeb(ctx, senderJID)
	}
}

func (h *Handler) resolveGroup(name string) (types.JID, error) {
	groups, err := h.db.GetGroupsByName(name)
	if err != nil {
		return types.JID{}, err
	}
	if len(groups) == 0 {
		return types.JID{}, fmt.Errorf("Command Aborted: Group '%s' not found. Ensure the bot is a member of the group.", name)
	}
	if len(groups) > 1 {
		return types.JID{}, fmt.Errorf("Command Aborted: Multiple groups found with name '%s'. Rename one of the groups to be unique before proceeding.", name)
	}
	jid, err := types.ParseJID(groups[0].JID)
	if err != nil {
		return types.JID{}, err
	}
	return jid, nil
}

func (h *Handler) sendPM(ctx context.Context, userJID types.JID, text string) {
	if err := h.client.SendPM(ctx, userJID, text); err != nil {
		slog.Error("failed to send PM", "user", userJID.String(), "error", err)
	}
}

// resolveSenderPN resolves a sender JID to its canonical phone-number JID string
// (e.g. "6283856883938@s.whatsapp.net"), matching how AdminJID is stored.
// Returns ok=false when a LID cannot be mapped to a phone number.
func (h *Handler) resolveSenderPN(ctx context.Context, senderJID types.JID) (string, bool) {
	cleanSender := senderJID.ToNonAD()
	if cleanSender.Server == types.HiddenUserServer {
		pnJID, err := h.client.WA.Store.LIDs.GetPNForLID(ctx, cleanSender)
		if err != nil || pnJID == types.EmptyJID {
			return "", false
		}
		return pnJID.String(), true
	}
	return cleanSender.String(), true
}

func (h *Handler) postSummaryMessage(ctx context.Context, groupJID types.JID, taskList *db.TaskList) {
	tasks, err := h.db.GetTasksByList(taskList.ID)
	if err != nil {
		return
	}
	text, mentions := format.FormatSummaryMessage(taskList, tasks)
	resp, err := h.client.SendGroupMessage(ctx, groupJID, text, mentions)
	if err != nil {
		return
	}
	h.db.SaveMessageMapping(resp.ID, taskList.ID, groupJID.String())
}
