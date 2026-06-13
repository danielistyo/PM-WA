package cmd

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) addList(ctx context.Context, senderJID types.JID, fields map[string]string) {
	name := fields["name"]
	waGroup := fields["wa-group"]

	if name == "" || waGroup == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\nadd-list\nname: {List Name}\nwa-group: {WA Group Name}")
		return
	}

	groupJID, err := h.resolveGroup(waGroup)
	if err != nil {
		h.sendPM(ctx, senderJID, err.Error())
		return
	}

	// Bot presence is confirmed by resolveGroup succeeding (group exists in wa_groups table).
	// The wa_groups table is cleaned when the bot is kicked via real-time events.

	senderInGroup, err := h.client.IsUserInGroup(ctx, groupJID, senderJID)
	if err == nil && !senderInGroup {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not a member of '"+waGroup+"'.")
		return
	}

	existing, _ := h.db.GetTaskListByNameAndGroup(name, groupJID.String())
	if existing != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: A Task List named '"+name+"' already exists in '"+waGroup+"'.")
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
			h.sendPM(ctx, senderJID, "Your account is currently in a state that cannot create Task Lists. Please ensure you are logged in on a primary device and try again. If the issue persists, contact support.")
			return
		}
	} else {
		// 2. It's already a standard phone number JID ("s.whatsapp.net")
		finalSenderStr = cleanSender.String()
	}

	_, err = h.db.CreateTaskList(name, groupJID.String(), finalSenderStr)
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	h.sendPM(ctx, senderJID, "Task List '"+name+"' created in '"+waGroup+"'. Add tasks with 'add-task', then activate with 'publish-list'.")
}
