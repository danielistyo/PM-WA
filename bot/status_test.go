package bot_test

import (
	"context"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"

	"pm-wa/bot"
)

func newStatusClient(t *testing.T) *bot.Client {
	t.Helper()
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3",
		"file:"+filepath.Join(t.TempDir(), "session.db")+"?_foreign_keys=on", waLog.Stdout("test", "ERROR", true))
	if err != nil {
		t.Fatalf("sqlstore.New() error: %v", err)
	}
	t.Cleanup(func() { _ = container.Close() })
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatalf("GetFirstDevice() error: %v", err)
	}
	return &bot.Client{WA: whatsmeow.NewClient(device, waLog.Stdout("test", "ERROR", true))}
}

func TestClientStatus(t *testing.T) {
	tests := []struct {
		name      string
		nilClient bool
		setDevice bool
		want      string
	}{
		{
			name:      "nil client reports unknown",
			nilClient: true,
			want:      bot.StatusUnknown,
		},
		{
			name: "no device id reports pairing required",
			want: bot.StatusPairingRequired,
		},
		{
			name:      "paired but not connected reports error",
			setDevice: true,
			want:      bot.StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c *bot.Client
			if !tt.nilClient {
				c = newStatusClient(t)
				if tt.setDevice {
					jid := types.JID{User: "628123", Server: types.DefaultUserServer}
					c.WA.Store.ID = &jid
				}
			}
			if got := c.Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestClientStatusConnected is the live-connect state; it requires a real
// linked session, so it only asserts the constant wiring here and relies on
// manual verification with a real number.
func TestClientStatusConstants(t *testing.T) {
	if bot.StatusConnected != "connected" ||
		bot.StatusPairingRequired != "pairing-required" ||
		bot.StatusError != "error" ||
		bot.StatusUnknown != "unknown" {
		t.Fatalf("status constants drifted; update badge rendering with them")
	}
}
