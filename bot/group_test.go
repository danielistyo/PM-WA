package bot_test

import (
	"testing"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
)

func TestMemberGroupJIDs(t *testing.T) {
	user := types.JID{User: "6000111222", Server: types.DefaultUserServer}
	other := "620003334444"

	tests := []struct {
		name    string
		groups  []types.GroupInfo
		user    types.JID
		want    map[string]bool
		wantLen int
	}{
		{
			name:    "nil groups yields empty map",
			groups:  nil,
			user:    user,
			want:    map[string]bool{},
			wantLen: 0,
		},
		{
			name:    "empty user yields empty map",
			groups:  []types.GroupInfo{{JID: types.JID{Server: types.GroupServer}}},
			user:    types.JID{},
			want:    map[string]bool{},
			wantLen: 0,
		},
		{
			name: "member found with phone-based participant",
			groups: []types.GroupInfo{
				{
					JID:          types.JID{User: "g1", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{JID: user}},
				},
			},
			user:    user,
			want:    map[string]bool{"g1@g.us": true},
			wantLen: 1,
		},
		{
			name: "member found with device-suffix JID via ToNonAD",
			groups: []types.GroupInfo{
				{
					JID: types.JID{User: "g2", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{
						JID: types.JID{User: user.User, Server: types.DefaultUserServer, Device: 2},
					}},
				},
			},
			user:    user,
			want:    map[string]bool{"g2@g.us": true},
			wantLen: 1,
		},
		{
			name: "non-member excluded",
			groups: []types.GroupInfo{
				{
					JID:          types.JID{User: "g3", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{JID: types.JID{User: other, Server: types.DefaultUserServer}}},
				},
			},
			user:    user,
			want:    map[string]bool{},
			wantLen: 0,
		},
		{
			name: "LID participant with matching phone number counts",
			groups: []types.GroupInfo{
				{
					JID: types.JID{User: "g4", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{
						JID:         types.JID{User: "999", Server: "lid"},
						PhoneNumber: user,
					}},
				},
			},
			user:    user,
			want:    map[string]bool{"g4@g.us": true},
			wantLen: 1,
		},
		{
			name: "LID-only group is member regardless of other phone numbers (parity with IsUserInGroup)",
			groups: []types.GroupInfo{
				{
					JID: types.JID{User: "g5", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{
						JID:         types.JID{User: "999", Server: "lid"},
						PhoneNumber: types.JID{User: other, Server: types.DefaultUserServer},
					}},
				},
			},
			user:    user,
			want:    map[string]bool{"g5@g.us": true},
			wantLen: 1,
		},
		{
			name: "LID-only group treated optimistically as member",
			groups: []types.GroupInfo{
				{
					JID: types.JID{User: "g6", Server: types.GroupServer},
					Participants: []types.GroupParticipant{
						{JID: types.JID{User: "111", Server: "lid"}},
						{JID: types.JID{User: "222", Server: "lid"}},
					},
				},
			},
			user:    user,
			want:    map[string]bool{"g6@g.us": true},
			wantLen: 1,
		},
		{
			name: "multiple groups: only member group returned",
			groups: []types.GroupInfo{
				{
					JID:          types.JID{User: "ga", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{JID: user}},
				},
				{
					JID:          types.JID{User: "gb", Server: types.GroupServer},
					Participants: []types.GroupParticipant{{JID: types.JID{User: other, Server: types.DefaultUserServer}}},
				},
			},
			user:    user,
			want:    map[string]bool{"ga@g.us": true},
			wantLen: 1,
		},
		{
			name: "isolated LID JID matches target user directly",
			groups: []types.GroupInfo{
				{
					JID: types.JID{User: "g7", Server: types.GroupServer},
					Participants: []types.GroupParticipant{
						{JID: types.JID{User: user.User, Server: "lid"}},
					},
				},
			},
			user:    user,
			want:    map[string]bool{"g7@g.us": true},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bot.MemberGroupJIDs(tt.groups, tt.user)
			if len(got) != tt.wantLen {
				t.Fatalf("len(got) = %d, want %d (got %v)", len(got), tt.wantLen, got)
			}
			for jid := range tt.want {
				if !got[jid] {
					t.Errorf("expected %q in result map, got %v", jid, got)
				}
			}
			for jid := range got {
				if !tt.want[jid] {
					t.Errorf("unexpected %q in result map", jid)
				}
			}
		})
	}
}
