package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	webTokenTTL        = 15 * time.Minute
	webRequestCooldown = time.Minute
)

var (
	webRateMu   sync.Mutex
	webLastSent = make(map[string]time.Time)
)

func (h *Handler) tasklistWeb(ctx context.Context, senderJID types.JID) {
	if h.webBaseURL == "" {
		h.sendPM(ctx, senderJID, "The web interface is not configured on this bot.")
		return
	}

	jid, ok := h.resolveSenderPN(ctx, senderJID)
	if !ok {
		h.sendPM(ctx, senderJID, "Command Aborted: Unable to verify your identity due to multi-device issues. Please ensure your account is properly linked and try again.")
		return
	}

	webRateMu.Lock()
	if last, exists := webLastSent[jid]; exists && time.Since(last) < webRequestCooldown {
		webRateMu.Unlock()
		h.sendPM(ctx, senderJID, "Please wait a moment before requesting another link.")
		return
	}
	webLastSent[jid] = time.Now()
	webRateMu.Unlock()

	token, err := randomToken()
	if err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	expiresAt := time.Now().Add(webTokenTTL).Unix()
	if err := h.db.CreateWebToken(token, jid, expiresAt); err != nil {
		h.sendPM(ctx, senderJID, "Internal error, please retry.")
		return
	}

	link := strings.TrimRight(h.webBaseURL, "/") + "/auth/" + token
	h.sendPM(ctx, senderJID, "Here is your private task manager link (valid for 15 minutes, single use):\n"+link)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
