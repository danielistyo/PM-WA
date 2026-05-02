package reply

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
	"pm-wa/db"
	"pm-wa/format"
)

type Handler struct {
	client *bot.Client
	db     *db.Database
}

func NewHandler(client *bot.Client, database *db.Database) *Handler {
	return &Handler{
		client: client,
		db:     database,
	}
}

func (h *Handler) HandleGroupReply(ctx context.Context, groupJID, senderJID, senderAlt types.JID, quotedID, text string) {
	taskListID, err := h.db.GetTaskListByMessageID(quotedID)
	if err != nil || taskListID == 0 {
		return
	}

	cmd := ParseReply(text)
	if cmd == nil {
		return
	}

	taskList, err := h.db.GetTaskList(taskListID)
	if err != nil {
		return
	}

	if taskList.Status == "stopped" {
		h.client.SendGroupReply(ctx, groupJID, "This task list has been stopped by the Admin. No further updates are accepted.")
		return
	}

	tasks, err := h.db.GetTasksByList(taskListID)
	if err != nil {
		return
	}

	for _, num := range cmd.Numbers {
		if !taskExistsAtPosition(tasks, num) {
			h.client.SendGroupReply(ctx, groupJID, "Invalid task number. Reply to the latest summary message for current task numbers.")
			return
		}
	}

	for _, num := range cmd.Numbers {
		task := getTaskAtPosition(tasks, num)
		if task == nil {
			continue
		}
		if !isAuthorized(senderJID, senderAlt, taskList, task) {
			h.client.SendGroupReply(ctx, groupJID, fmt.Sprintf("Unauthorized: You are not the assignee for Task #%d. Update failed.", num))
			return
		}
	}

	targetStatus := "done"
	if cmd.Action == "todo" {
		targetStatus = "todo"
	}

	allAlready := true
	for _, num := range cmd.Numbers {
		task := getTaskAtPosition(tasks, num)
		if task != nil && task.Status != targetStatus {
			allAlready = false
			break
		}
	}
	if allAlready {
		return
	}

	tx, err := h.db.BeginTx()
	if err != nil {
		return
	}
	for _, num := range cmd.Numbers {
		if err := h.db.UpdateTaskStatusTx(tx, taskListID, num, targetStatus); err != nil {
			tx.Rollback()
			return
		}
	}
	if err := tx.Commit(); err != nil {
		return
	}

	updatedTasks, _ := h.db.GetTasksByList(taskListID)
	text2, mentions := format.FormatSummaryMessage(taskList, updatedTasks)
	resp, err := h.client.SendGroupMessage(ctx, groupJID, text2, mentions)
	if err == nil {
		h.db.SaveMessageMapping(resp.ID, taskList.ID, groupJID.String())
	}
}

func taskExistsAtPosition(tasks []db.Task, position int) bool {
	for _, t := range tasks {
		if t.Position == position {
			return true
		}
	}
	return false
}

func getTaskAtPosition(tasks []db.Task, position int) *db.Task {
	for i := range tasks {
		if tasks[i].Position == position {
			return &tasks[i]
		}
	}
	return nil
}

func isAuthorized(senderJID, senderAlt types.JID, taskList *db.TaskList, task *db.Task) bool {
	senderUser := senderJID.ToNonAD().User
	senderAltUser := ""
	if !senderAlt.IsEmpty() {
		senderAltUser = senderAlt.ToNonAD().User
	}

	adminJID, _ := types.ParseJID(taskList.AdminJID)
	if adminJID.User == senderUser || (senderAltUser != "" && adminJID.User == senderAltUser) {
		return true
	}

	for _, a := range task.Assignees {
		aJID, _ := types.ParseJID(a.AssigneeJID)
		if a.LeftGroup {
			continue
		}
		if aJID.User == senderUser || (senderAltUser != "" && aJID.User == senderAltUser) {
			return true
		}
	}
	return false
}
