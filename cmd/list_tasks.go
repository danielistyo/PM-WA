package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) listTasks(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]

	if waGroup == "" || listName == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\nlist-tasks\nwa-group: {WA Group}\nlist: {List Name}")
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

	taskList, err := h.db.GetTaskListByNameAndGroup(listName, groups[0].JID)
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+listName+"' not found in '"+waGroup+"'.")
		return
	}

	if taskList.AdminJID != senderJID.ToNonAD().String() {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not the owner of '"+listName+"'.")
		return
	}

	tasks, err := h.db.GetTasksByList(taskList.ID)
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	if len(tasks) == 0 {
		h.sendPM(ctx, senderJID, "Task List '"+listName+"' has no tasks.")
		return
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tasks in '%s' [%s]:\n\n", listName, taskList.Status))

	for _, t := range tasks {
		status := "Todo"
		if t.Status == "done" {
			status = "Done"
		}
		deadline := time.Unix(t.Deadline, 0).In(gmt7).Format("2006-01-02 15:04")
		reminderFlag := "yes"
		if !t.Reminder {
			reminderFlag = "no"
		}
		var assignees []string
		for _, a := range t.Assignees {
			label := a.Phone()
			if a.LeftGroup {
				label += " (left)"
			}
			assignees = append(assignees, label)
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n   Assignees: %s\n   Deadline: %s\n   Reminder: %s\n\n",
			t.Position, status, t.Title, strings.Join(assignees, ", "), deadline, reminderFlag))
	}

	h.sendPM(ctx, senderJID, b.String())
}
