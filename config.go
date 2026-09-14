package main

import (
	"os"
)

type Config struct {
	DBPath        string
	SessionDBPath string
	ScheduleTime  string
	WebBaseURL    string
	WebListenAddr string
}

func DefaultConfig() Config {
	scheduleTime := os.Getenv("SCHEDULE_TIME")
	if scheduleTime == "" {
		scheduleTime = "0 8 * * *"
	}
	webListenAddr := os.Getenv("WEB_LISTEN_ADDR")
	if webListenAddr == "" {
		webListenAddr = ":8080"
	}
	return Config{
		DBPath:        "pm-wa.db",
		SessionDBPath: "wa-session.db",
		ScheduleTime:  scheduleTime,
		WebBaseURL:    os.Getenv("WEB_BASE_URL"),
		WebListenAddr: webListenAddr,
	}
}
