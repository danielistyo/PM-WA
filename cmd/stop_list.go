package cmd

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) stopList(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]

	if waGroup == "" || listName == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\nstop-list\nwa-group: {WA Group}\nlist: {List Name}")
		return
	}

	groupJID, err := h.resolveGroup(waGroup)
	if err != nil {
		h.sendPM(ctx, senderJID, err.Error())
		return
	}

	taskList, err := h.db.GetTaskListByNameAndGroup(listName, groupJID.String())
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' not found in '"+waGroup+"'.")
		return
	}

	if taskList.AdminJID != senderJID.ToNonAD().String() {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not the owner of '"+listName+"'.")
		return
	}

	h.db.UpdateListStatus(taskList.ID, "stopped")
	h.sendPM(ctx, senderJID, "Task List '"+listName+"' has been permanently stopped. No further reminders will be sent. This cannot be undone.")
}
