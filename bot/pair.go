package bot

import (
	"context"
	"fmt"
	"os"

	"github.com/mdp/qrterminal/v3"
)

// PairNew walks the QR linking flow for an unpaired client: it prints QR
// codes to stdout and blocks until the account is paired, times out, or
// fails. It returns nil only when pairing succeeded. The plain Connect (used
// by the primary account at boot) is unchanged; PairNew is for the
// `pm-wa pair <name>` subcommand.
func (c *Client) PairNew() error {
	if c.WA.Store.ID != nil {
		return fmt.Errorf("cannot pair: session store already contains a device")
	}
	qrChan, err := c.WA.GetQRChannel(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start QR pairing: %w", err)
	}
	if err := c.WA.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	for evt := range qrChan {
		switch {
		case evt.Event == "code":
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			fmt.Println("Scan the QR code above with your WhatsApp app (Settings > Linked devices).")
		case evt.Event == "success":
			return nil
		case evt.Error != nil:
			return fmt.Errorf("pairing failed: %w", evt.Error)
		default:
			return fmt.Errorf("pairing failed: %s", evt.Event)
		}
	}
	return fmt.Errorf("pairing failed: QR channel closed before pairing completed")
}

// ResetSession removes the linked device from the session store so the
// account can be paired again, and closes the store container. It is a no-op
// when the store has no device.
func (c *Client) ResetSession() error {
	if c.WA.Store.ID == nil {
		return nil
	}
	if err := c.container.DeleteDevice(context.Background(), c.WA.Store); err != nil {
		return fmt.Errorf("failed to delete stored device: %w", err)
	}
	c.Disconnect()
	return c.container.Close()
}

// CloseStore closes the session store container. Thin glue for the pair
// subcommand when it reopens a fresh store after a reset.
func (c *Client) CloseStore() error {
	return c.container.Close()
}
