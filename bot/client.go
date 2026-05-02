package bot

import (
	"context"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
)

type Client struct {
	WA        *whatsmeow.Client
	container *sqlstore.Container
}

func NewClient(sessionDBPath string) (*Client, error) {
	logger := waLog.Stdout("Bot", "WARN", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+sessionDBPath+"?_foreign_keys=on", logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Stdout("Client", "WARN", true))

	return &Client{
		WA:        client,
		container: container,
	}, nil
}

func (c *Client) Connect() error {
	if c.WA.Store.ID == nil {
		qrChan, _ := c.WA.GetQRChannel(context.Background())
		if err := c.WA.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("Scan the QR code above with your WhatsApp app")
			}
		}
	} else {
		if err := c.WA.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}
	return nil
}

func (c *Client) Disconnect() {
	c.WA.Disconnect()
}
