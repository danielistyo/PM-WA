package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// bulkTimeout bounds the entire add-tasks command: one participants RPC plus a
// single batched insert finishes well within it (observed < 2s), keeping the
// chat reply snappy and clear of the 15s web WriteTimeout figure.
const bulkTimeout = 5 * time.Second

func (h *Handler) addTasks(ctx context.Context, senderJID types.JID, text string) {
	ctx, cancel := context.WithTimeout(ctx, bulkTimeout)
	defer cancel()

	payload, err := ParseBulkPayload(text)
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: "+err.Error())
		return
	}

	if len(payload.Tasks) == 0 {
		h.sendPM(ctx, senderJID, "Command Aborted: No task blocks found. "+bulkUsage)
		return
	}
	if len(payload.Tasks) > maxBulkTasks {
		h.sendPM(ctx, senderJID, bulkCapReply(len(payload.Tasks)))
		return
	}
	if payload.WaGroup == "" || payload.ListName == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. "+bulkUsage)
		return
	}

	groupJID, err := h.resolveGroup(payload.WaGroup)
	if err != nil {
		h.sendPM(ctx, senderJID, err.Error())
		return
	}

	taskList, err := h.db.GetTaskListByNameAndGroup(payload.ListName, groupJID.String())
	if err != nil {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+payload.ListName+"' not found in '"+payload.WaGroup+"'.")
		return
	}

	finalSenderStr, ok := h.resolveSenderPN(ctx, senderJID)
	if !ok {
		h.sendPM(ctx, senderJID, "Command Aborted: Unable to verify your identity due to multi-device issues. Please ensure your account is properly linked and try again.")
		return
	}
	if taskList.AdminJID != finalSenderStr {
		h.sendPM(ctx, senderJID, "Command Aborted: You are not the owner of '"+payload.ListName+"'.")
		return
	}
	if taskList.Status == "stopped" {
		h.sendPM(ctx, senderJID, "Command Aborted: Task List '"+payload.ListName+"' has been permanently stopped.")
		return
	}

	existingTitles, err := h.db.GetTaskTitlesByList(taskList.ID)
	if err != nil {
		slog.Error("add-tasks: failed to load task titles", "list", taskList.ID, "error", err)
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	// One participants RPC for the whole batch; membership is checked in memory.
	participants, _, lidPrimary, err := h.client.GetGroupParticipantsEx(ctx, groupJID)
	if err != nil {
		slog.Error("add-tasks: failed to fetch participants", "group", groupJID.String(), "error", err)
		h.sendPM(ctx, senderJID, "Command Aborted: Could not fetch group members for validation. Please retry.")
		return
	}
	// Parity with add-task's IsUserInGroup quirk: LID-primary groups cannot be
	// verified by phone-keyed membership, so the check is skipped optimistically.
	if lidPrimary {
		slog.Warn("add-tasks: group uses LID-only participants, skipping membership validation", "group", groupJID.String())
	}

	bulkTasks, errs := validateBulkTasks(payload.WaGroup, payload.ListName, payload, existingTitles, participants, !lidPrimary)
	if len(errs) > 0 {
		h.sendPM(ctx, senderJID, "Command Aborted: Nothing was added.\n"+strings.Join(errs, "\n"))
		return
	}

	if _, err := h.db.CreateTasks(taskList.ID, bulkRows(bulkTasks)); err != nil {
		slog.Error("add-tasks: batch insert failed", "list", taskList.ID, "count", len(bulkTasks), "error", err)
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	h.sendPM(ctx, senderJID, fmt.Sprintf("Added %d tasks to '%s'.", len(bulkTasks), payload.ListName))
}
