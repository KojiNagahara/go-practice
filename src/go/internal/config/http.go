package config

import (
	"time"
)

type HTTPSettings struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func loadHTTPSettings() HTTPSettings {
	return HTTPSettings{
		ReadHeaderTimeout: durationOrDefault("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		ReadTimeout:       durationOrDefault("HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:      durationOrDefault("HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:       durationOrDefault("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   durationOrDefault("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func durationOrDefault(key string, fallback time.Duration) time.Duration {
	value := valueOrDefault(key, "")
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return fallback
	}
	return duration
}
