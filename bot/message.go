package bot

import (
	"context"
	"fmt"
	"log/slog"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type SendResponse struct {
	ID string
}

func (c *Client) SendGroupMessage(ctx context.Context, groupJID types.JID, text string, mentionJIDs []string) (*SendResponse, error) {
	var msg *waE2E.Message
	if len(mentionJIDs) > 0 {
		msg = &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
				ContextInfo: &waE2E.ContextInfo{
					MentionedJID: mentionJIDs,
				},
			},
		}
	} else {
		msg = &waE2E.Message{
			Conversation: proto.String(text),
		}
	}

	resp, err := c.WA.SendMessage(ctx, groupJID, msg)
	if err != nil {
		slog.Error("failed to send group message", "group", groupJID.String(), "error", err)
		return nil, err
	}
	return &SendResponse{ID: resp.ID}, nil
}

func (c *Client) SendPM(ctx context.Context, userJID types.JID, text string) error {
	recipient := userJID.ToNonAD()
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	_, err := c.WA.SendMessage(ctx, recipient, msg)
	if err != nil {
		slog.Error("failed to send PM", "user", recipient.String(), "error", err)
	}
	return err
}

func (c *Client) SendGroupReply(ctx context.Context, groupJID types.JID, text string) error {
	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	_, err := c.WA.SendMessage(ctx, groupJID, msg)
	if err != nil {
		slog.Error("failed to send group reply", "group", groupJID.String(), "error", err)
	}
	return err
}

func PhoneToJID(phone string) types.JID {
	return types.JID{
		User:   phone,
		Server: types.DefaultUserServer,
	}
}

func JIDToPhone(jid types.JID) string {
	return jid.ToNonAD().User
}

func FormatJIDString(phone string) string {
	return fmt.Sprintf("%s@%s", phone, types.DefaultUserServer)
}
