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

type MemberInfo struct {
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

func (c *Client) GetGroupPhoneMembers(ctx context.Context, groupJID types.JID) ([]MemberInfo, error) {
	groupInfo, err := c.WA.GetGroupInfo(ctx, groupJID)
	if err != nil {
		return nil, err
	}
	var members []MemberInfo
	for _, p := range groupInfo.Participants {
		var userJID types.JID
		if p.JID.Server != "lid" {
			userJID = p.JID.ToNonAD()
		} else if !p.PhoneNumber.IsEmpty() {
			userJID = p.PhoneNumber.ToNonAD()
		} else {
			continue
		}
		
		phone := userJID.User
		name := phone // fallback
		
		// Attempt to get contact info
		if contact, err := c.WA.Store.Contacts.GetContact(ctx, userJID); err == nil && contact.Found {
			if contact.FullName != "" {
				name = contact.FullName
			} else if contact.PushName != "" {
				name = contact.PushName
			} else if contact.BusinessName != "" {
				name = contact.BusinessName
			}
		}
		
		members = append(members, MemberInfo{Phone: phone, Name: name})
	}
	return members, nil
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

// MemberGroupJIDs computes the set of group JIDs (as strings) in which userJID is a
// participant, from pre-fetched group infos. Groups whose participants are LID-only
// (no phone-based JIDs available) are treated optimistically as membership matches,
// mirroring IsUserInGroup.
func MemberGroupJIDs(groups []types.GroupInfo, userJID types.JID) map[string]bool {
	member := make(map[string]bool, len(groups))
	target := userJID.ToNonAD().User
	if target == "" {
		return member
	}
	for _, g := range groups {
		hasLIDOnly := true
		matched := false
		for _, p := range g.Participants {
			if p.JID.Server != "lid" {
				hasLIDOnly = false
			}
			if p.JID.ToNonAD().User == target {
				matched = true
			}
			if !p.PhoneNumber.IsEmpty() && p.PhoneNumber.ToNonAD().User == target {
				matched = true
			}
		}
		if matched {
			member[g.JID.String()] = true
		} else if hasLIDOnly {
			slog.Warn("group uses LID-only participants, skipping membership check", "group", g.JID.String())
			member[g.JID.String()] = true
		}
	}
	return member
}

// GetMemberGroups fetches all joined groups of the bot in a single RPC and returns the
// set of group JIDs (as strings) where userJID is a participant. Use this instead of
// calling IsUserInGroup once per group.
func (c *Client) GetMemberGroups(ctx context.Context, userJID types.JID) (map[string]bool, error) {
	groups, err := c.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	return MemberGroupJIDs(groups, userJID), nil
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
