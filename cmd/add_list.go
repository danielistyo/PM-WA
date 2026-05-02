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

	_, err = h.db.CreateTaskList(name, groupJID.String(), senderJID.ToNonAD().String())
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	h.sendPM(ctx, senderJID, "Task List '"+name+"' created in '"+waGroup+"'. Add tasks with 'add-task', then activate with 'publish-list'.")
}
