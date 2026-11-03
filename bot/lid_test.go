package bot_test

import (
	"testing"

	"go.mau.fi/whatsmeow/types"

	"pm-wa/bot"
)

func TestLIDPrimaryOnly(t *testing.T) {
	tests := []struct {
		name         string
		participants []types.GroupParticipant
		want         bool
	}{
		{
			name:         "nil participants is trivially LID-only",
			participants: nil,
			want:         true,
		},
		{
			name: "empty participants is trivially LID-only",
			participants: []types.GroupParticipant{},
			want:         true,
		},
		{
			name: "all JIDs on lid server",
			participants: []types.GroupParticipant{
				{JID: types.JID{User: "111", Server: types.HiddenUserServer}},
				{JID: types.JID{User: "222", Server: types.HiddenUserServer}},
			},
			want: true,
		},
		{
			name: "one phone-based JID makes it not LID-only",
			participants: []types.GroupParticipant{
				{JID: types.JID{User: "111", Server: types.HiddenUserServer}},
				{JID: types.JID{User: "6281200000000", Server: types.DefaultUserServer}},
			},
			want: false,
		},
		{
			name: "LID JID with resolvable phone number still counts as LID-primary",
			participants: []types.GroupParticipant{
				{
					JID:         types.JID{User: "111", Server: types.HiddenUserServer},
					PhoneNumber: types.JID{User: "6281200000000", Server: types.DefaultUserServer},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bot.LIDPrimaryOnly(tt.participants); got != tt.want {
				t.Fatalf("LIDPrimaryOnly() = %v, want %v", got, tt.want)
			}
		})
	}
}
