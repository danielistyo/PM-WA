package cmd

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) publishList(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]

	if waGroup == "" || listName == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\npublish-list\nwa-group: {WA Group}\nlist: {List Name}")
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

	if taskList.Status == "stopped" {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' has been permanently stopped and cannot be published again.")
		return
	}

	taskCount, _ := h.db.GetTaskCountByList(taskList.ID)
	if taskCount == 0 {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' has no tasks. Add at least one task with 'add-task' before publishing.")
		return
	}

	if taskList.Status == "unpublished" {
		h.db.UpdateListStatus(taskList.ID, "active")
		taskList.Status = "active"
	}

	h.postSummaryMessage(ctx, groupJID, taskList)
}
