package cmd

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func (h *Handler) listLists(ctx context.Context, senderJID types.JID, fields map[string]string) {
	waGroup := fields["wa-group"]

	if waGroup == "" {
		h.sendPM(ctx, senderJID, "Command Aborted: Missing required fields. Usage:\nlist-lists\nwa-group: {WA Group Name}")
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
			h.sendPM(ctx, senderJID, "Your account is currently in a state that cannot list Task Lists. Please ensure you are logged in on a primary device and try again. If the issue persists, contact support.")
			return
		}
	} else {
		// 2. It's already a standard phone number JID ("s.whatsapp.net")
		finalSenderStr = cleanSender.String()
	}

	lists, err := h.db.GetListsByGroupAndAdmin(groups[0].JID, finalSenderStr)
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	if len(lists) == 0 {
		h.sendPM(ctx, senderJID, "You have no Task Lists in '"+waGroup+"'.")
		return
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Your Task Lists in '%s':\n\n", waGroup))
	for _, l := range lists {
		taskCount, _ := h.db.GetTaskCountByList(l.ID)
		b.WriteString(fmt.Sprintf("• %s [%s] (%d tasks)\n", l.Name, l.Status, taskCount))
	}

	h.sendPM(ctx, senderJID, b.String())
}
