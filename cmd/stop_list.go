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

	// Strip multi-device suffixes first
	cleanSender := senderJID.ToNonAD()
	var finalSenderStr string

	// 1. Check if the sender's domain server is explicitly a LID
	if cleanSender.Server == types.HiddenUserServer { // "lid"
		// It's a LID, so try to look up the associated Phone Number JID
		pnJID, err := h.client.WA.Store.LIDs.GetPNForLID(ctx, cleanSender)

		// If no database error occurred and the returned JID is valid, use it
		if err == nil && pnJID != types.EmptyJID {
			finalSenderStr = pnJID.String() // Safely becomes "6283856883938@s.whatsapp.net"
		} else {
			// Fallback to the LID string if lookup fails or isn't cached yet
			finalSenderStr = cleanSender.String()
		}
	} else {
		// 2. It's already a standard phone number JID ("s.whatsapp.net")
		finalSenderStr = cleanSender.String()
	}

	// 3. Perform a single, secure comparison check
	if taskList.AdminJID != finalSenderStr {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not the owner of '"+listName+"'.")
		return
	}

	h.db.UpdateListStatus(taskList.ID, "stopped")
	h.sendPM(ctx, senderJID, "Task List '"+listName+"' has been permanently stopped. No further reminders will be sent. This cannot be undone.")
}
