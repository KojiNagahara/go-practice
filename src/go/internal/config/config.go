package config

import "os"

type Settings struct {
	Port          string
	Environment   string
	DatabaseURL   string
	RedisURL      string
	StorageDriver string
	UploadDir     string
	S3Bucket      string
	OAuth         OAuthSettings
	HTTP          HTTPSettings
}

type OAuthSettings struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	AmazonClientID     string
	AmazonClientSecret string
	AmazonRedirectURL  string
}

func (settings Settings) HasDatabase() bool {
	return settings.DatabaseURL != ""
}

func Load() Settings {
	return Settings{
		Port:          valueOrDefault("PORT", "8080"),
		Environment:   valueOrDefault("APP_ENV", "development"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
		StorageDriver: valueOrDefault("STORAGE_DRIVER", "local"),
		UploadDir:     valueOrDefault("UPLOAD_DIR", "/data/uploads"),
		S3Bucket:      os.Getenv("S3_BUCKET"),
		OAuth: OAuthSettings{
			GoogleClientID: os.Getenv("GOOGLE_OAUTH_CLIENT_ID"), GoogleClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"), GoogleRedirectURL: os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
			AmazonClientID: os.Getenv("AMAZON_OAUTH_CLIENT_ID"), AmazonClientSecret: os.Getenv("AMAZON_OAUTH_CLIENT_SECRET"), AmazonRedirectURL: os.Getenv("AMAZON_OAUTH_REDIRECT_URL"),
		},
		HTTP: loadHTTPSettings(),
	}
}

func valueOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
