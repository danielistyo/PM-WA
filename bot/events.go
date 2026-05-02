package bot

import (
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

func ExtractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if msg.GetExtendedTextMessage() != nil {
		return msg.GetExtendedTextMessage().GetText()
	}
	return ""
}

func GetQuotedMessageID(evt *events.Message) string {
	if evt.Message == nil {
		return ""
	}
	extMsg := evt.Message.GetExtendedTextMessage()
	if extMsg == nil {
		return ""
	}
	ctxInfo := extMsg.GetContextInfo()
	if ctxInfo == nil {
		return ""
	}
	return ctxInfo.GetStanzaID()
}
