package main

type Config struct {
	DBPath        string
	SessionDBPath string
}

func DefaultConfig() Config {
	return Config{
		DBPath:        "pm-wa.db",
		SessionDBPath: "wa-session.db",
	}
}
