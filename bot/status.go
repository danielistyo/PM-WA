package bot

// Status values surfaced by the web UI connection badge (T5).
const (
	StatusConnected       = "connected"
	StatusPairingRequired = "pairing-required"
	StatusError           = "error"
	StatusUnknown         = "unknown"
)

// Status reports the client's connection state for the UI badge. The state is
// derived from live whatsmeow state on every call, so a reconnect flips the
// badge on the next render without extra bookkeeping. A nil client or client
// without an inner whatsmeow client yields unknown so rendering degrades
// gracefully instead of failing.
func (c *Client) Status() string {
	if c == nil || c.WA == nil {
		return StatusUnknown
	}
	if c.WA.Store.ID == nil {
		return StatusPairingRequired
	}
	if c.WA.IsConnected() {
		return StatusConnected
	}
	return StatusError
}
