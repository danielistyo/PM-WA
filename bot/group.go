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

func (c *Client) GetGroupParticipants(ctx context.Context, groupJID types.JID) (map[string]bool, error) {
	groupInfo, err := c.WA.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return nil, err
	}
	participants := make(map[string]bool)
	for _, p := range groupInfo.Participants {
		participants[p.JID.ToNonAD().User] = true
	}
	return participants, nil
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
