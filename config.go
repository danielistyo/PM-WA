package main

import "os"

type Config struct {
	DBPath        string
	SessionDBPath string
	ScheduleTime  string
}

func DefaultConfig() Config {
	scheduleTime := os.Getenv("SCHEDULE_TIME")
	if scheduleTime == "" {
		scheduleTime = "0 8 * * *"
	}
	return Config{
		DBPath:        "pm-wa.db",
		SessionDBPath: "wa-session.db",
		ScheduleTime:  scheduleTime,
	}
}
