package cmd

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) deleteList(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]

	if waGroup == "" || listName == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\ndelete-list\nwa-group: {WA Group}\nlist: {List Name}")
		return
	}

	groups, err := h.db.GetGroupsByName(waGroup)
	if err != nil || len(groups) == 0 {
		h.sendPM(ctx, senderJID, fmt.Sprintf("Command Aborted: Group '%s' not found.", waGroup))
		return
	}
	if len(groups) > 1 {
		h.sendPM(ctx, senderJID, fmt.Sprintf("Command Aborted: Multiple groups found with name '%s'. Rename one of the groups to be unique before proceeding.", waGroup))
		return
	}

	groupJIDStr := groups[0].JID

	taskList, err := h.db.GetTaskListByNameAndGroup(listName, groupJIDStr)
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' not found in '"+waGroup+"'.")
		return
	}

	if taskList.AdminJID != senderJID.ToNonAD().String() {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not the owner of '"+listName+"'.")
		return
	}

	taskCount, _ := h.db.GetTaskCountByList(taskList.ID)

	h.db.DeleteMessageMapByList(taskList.ID)
	h.db.DeleteTaskList(taskList.ID)

	groupJID, _ := types.ParseJID(groupJIDStr)
	botInGroup, _ := h.client.IsBotInGroup(ctx, groupJID)
	if !botInGroup {
		h.sendPM(ctx, senderJID, fmt.Sprintf("Note: Bot is no longer a member of '%s'. Task List '%s' and all %d tasks have been permanently deleted.", waGroup, listName, taskCount))
	} else {
		h.sendPM(ctx, senderJID, fmt.Sprintf("Task List '%s' and all %d tasks have been permanently deleted.", listName, taskCount))
	}
}
