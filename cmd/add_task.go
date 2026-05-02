package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
)

var gmt7 = time.FixedZone("GMT+7", 7*60*60)

func (h *Handler) addTask(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]
	listName := fields["list"]
	title := fields["title"]
	assignStr := fields["assign"]
	deadlineStr := fields["deadline"]
	reminderStr := fields["reminder"]

	if waGroup == "" || listName == "" || title == "" || assignStr == "" || deadlineStr == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\nadd-task\nwa-group: {WA Group}\nlist: {List Name}\ntitle: {Title}\nassign: {Phone1, Phone2}\ndeadline: {YYYY-MM-DD HH:MM}\nreminder: {yes/no}")
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

	phones := parsePhoneList(assignStr)
	if len(phones) == 0 {
		h.sendPM(ctx, senderJID, "Command Aborted: No valid assignee phone numbers provided.")
		return
	}

	var assigneeJIDs []string
	for _, phone := range phones {
		userJID := bot.PhoneToJID(phone)
		inGrp, err := h.client.IsUserInGroup(ctx, groupJID, userJID)
		if err != nil || !inGrp {
			h.sendPM(ctx, senderJID, fmt.Sprintf("Command Aborted: Assignee %s is not a member of '%s'. Add them first.", phone, waGroup))
			return
		}
		assigneeJIDs = append(assigneeJIDs, bot.FormatJIDString(phone))
	}

	deadline, err := time.ParseInLocation("2006-01-02 15:04", deadlineStr, gmt7)
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: Invalid deadline format. Use YYYY-MM-DD HH:MM.")
		return
	}

	reminder := strings.ToLower(strings.TrimSpace(reminderStr)) != "no"

	_, err = h.db.CreateTask(taskList.ID, title, deadline.Unix(), reminder, assigneeJIDs)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			h.sendPM(ctx, senderJID, "Command Aborted: A task with title '"+title+"' already exists in '"+listName+"'.")
			return
		}
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	h.sendPM(ctx, senderJID, "Task '"+title+"' added to '"+listName+"'.")
}

func parsePhoneList(s string) []string {
	var phones []string
	for _, part := range strings.Split(s, ",") {
		phone := strings.TrimSpace(part)
		if phone != "" {
			phones = append(phones, phone)
		}
	}
	return phones
}
