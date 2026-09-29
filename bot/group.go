package bot

import (
	"context"
	"log/slog"

	"go.mau.fi/whatsmeow/types"
)

func (c *Client) IsUserInGroup(ctx context.Context, groupJID, userJID types.JID) (bool, error) {
	groupInfo, err := c.WA.GetGroupInfo(ctx, groupJID)
	if err != nil {
		slog.Error("GetGroupInfo failed", "group", groupJID.String(), "error", err)
		return false, err
	}
	targetUser := userJID.ToNonAD().User
	hasLIDOnly := true
	for _, p := range groupInfo.Participants {
		if p.JID.Server != "lid" {
			hasLIDOnly = false
		}
		if p.JID.ToNonAD().User == targetUser {
			return true, nil
		}
	}
	if hasLIDOnly {
		slog.Warn("group uses LID-only participants, skipping membership check", "group", groupJID.String())
		return true, nil
	}
	return false, nil
}

func (c *Client) IsBotInGroup(ctx context.Context, groupJID types.JID) (bool, error) {
	if c.WA.Store.ID == nil {
		return false, nil
	}
	return c.IsUserInGroup(ctx, groupJID, *c.WA.Store.ID)
}

func (c *Client) GetGroupPhoneMembers(ctx context.Context, groupJID types.JID) ([]string, error) {
	groupInfo, err := c.WA.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return nil, err
	}
	var phones []string
	for _, p := range groupInfo.Participants {
		// Prefer the Phone-based JID if it's available, otherwise fallback to the primary JID
		// Note that non-LID JIDs are typically phone numbers. 
		var user string
		if p.JID.Server != "lid" {
			user = p.JID.ToNonAD().User
		} else if !p.PhoneNumber.IsEmpty() {
			user = p.PhoneNumber.ToNonAD().User
		} else {
			// fallback for LID only but try to extract phone? In LID-only groups, they might not have phone numbers.
			// But for now, we just skip LIDs that don't have phone numbers as we need phone numbers for assignments.
			continue
		}
		phones = append(phones, user)
	}
	return phones, nil
}

func (c *Client) GetGroupParticipantsEx(ctx context.Context, groupJID types.JID) (map[string]bool, bool, error) {
	groupInfo, err := c.WA.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return nil, false, err
	}
	participants := make(map[string]bool)
	hasPhoneJIDs := false
	for _, p := range groupInfo.Participants {
		if p.JID.Server != "lid" {
			hasPhoneJIDs = true
		}
		// Always index by primary JID user
		participants[p.JID.ToNonAD().User] = true
		// Also index by phone number user when available (handles LID-primary groups
		// where the primary JID is a LID but we store phone-based JIDs for assignees)
		if !p.PhoneNumber.IsEmpty() {
			participants[p.PhoneNumber.ToNonAD().User] = true
			hasPhoneJIDs = true
		}
	}
	return participants, hasPhoneJIDs, nil
}

func (c *Client) GetJoinedGroups(ctx context.Context) ([]types.GroupInfo, error) {
	groups, err := c.WA.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]types.GroupInfo, len(groups))
	for i, g := range groups {
		result[i] = *g
	}
	return result, nil
}

func (c *Client) BotJID() types.JID {
	if c.WA.Store.ID == nil {
		return types.JID{}
	}
	return c.WA.Store.ID.ToNonAD()
}
