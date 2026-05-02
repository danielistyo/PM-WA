package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) deleteTask(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]
	taskNumStr := fields["task"]

	if waGroup == "" || listName == "" || taskNumStr == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\ndelete-task\nwa-group: {WA Group}\nlist: {List Name}\ntask: {Number}")
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
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' has been permanently stopped.")
		return
	}

	taskNum, err := strconv.Atoi(strings.TrimSpace(taskNumStr))
	if err != nil || taskNum < 1 {
		h.sendPM(ctx, senderJID, "Command Aborted: Invalid task number.")
		return
	}

	if !h.db.TaskExistsAtPosition(taskList.ID, taskNum) {
		h.sendPM(ctx, senderJID, "Command Aborted: Task number does not exist.")
		return
	}

	title, err := h.db.DeleteTask(taskList.ID, taskNum)
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	h.sendPM(ctx, senderJID, fmt.Sprintf("Task #%d ('%s') deleted from '%s'. Tasks have been re-numbered.", taskNum, title, listName))

	if taskList.Status == "active" {
		h.postSummaryMessage(ctx, groupJID, taskList)
	}
}
