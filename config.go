package main

import (
	"os"
)

type Config struct {
	DBPath        string
	SessionDBPath string
	WebBaseURL    string
	WebListenAddr string
}

func DefaultConfig() Config {
	webListenAddr := os.Getenv("WEB_LISTEN_ADDR")
	if webListenAddr == "" {
		webListenAddr = ":8080"
	}
	return Config{
		DBPath:        "pm-wa.db",
		SessionDBPath: "wa-session.db",
		WebBaseURL:    os.Getenv("WEB_BASE_URL"),
		WebListenAddr: webListenAddr,
	}
}
