package main

import (
	"os"
)

type Config struct {
	WebBaseURL    string
	WebListenAddr string
}

func DefaultConfig() Config {
	webListenAddr := os.Getenv("WEB_LISTEN_ADDR")
	if webListenAddr == "" {
		webListenAddr = ":8080"
	}
	return Config{
		WebBaseURL:    os.Getenv("WEB_BASE_URL"),
		WebListenAddr: webListenAddr,
	}
}
