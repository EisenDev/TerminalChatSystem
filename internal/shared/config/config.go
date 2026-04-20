package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Server struct {
	HTTPAddr       string
	DatabaseURL    string
	RedisAddr      string
	RedisUser      string
	RedisPassword  string
	RedisDB        int
	AllowedOrigin  string
	DefaultChannel string
	HistoryLimit   int
	LogFormat      string
	PingInterval   time.Duration
	WriteTimeout   time.Duration
	ReadLimitBytes int64
}

type Client struct {
	ServerURL      string
	Workspace      string
	DefaultHandle  string
	DefaultChannel string
	LogFormat      string
	ReconnectDelay time.Duration
}

func LoadServer() (Server, error) {
	cfg := Server{
		HTTPAddr:       getEnv("CHAT_HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("CHAT_DATABASE_URL"),
		RedisAddr:      os.Getenv("CHAT_REDIS_ADDR"),
		RedisUser:      os.Getenv("CHAT_REDIS_USER"),
		RedisPassword:  os.Getenv("CHAT_REDIS_PASSWORD"),
		RedisDB:        getEnvInt("CHAT_REDIS_DB", 0),
		AllowedOrigin:  getEnv("CHAT_ALLOWED_ORIGIN", "*"),
		DefaultChannel: getEnv("CHAT_DEFAULT_CHANNEL", "lobby"),
		HistoryLimit:   getEnvInt("CHAT_HISTORY_LIMIT", 50),
		LogFormat:      getEnv("CHAT_LOG_FORMAT", "text"),
		PingInterval:   getEnvDuration("CHAT_PING_INTERVAL", 25*time.Second),
		WriteTimeout:   getEnvDuration("CHAT_WRITE_TIMEOUT", 10*time.Second),
		ReadLimitBytes: int64(getEnvInt("CHAT_WS_READ_LIMIT", 1024*1024)),
	}
	if cfg.DatabaseURL == "" {
		return Server{}, fmt.Errorf("CHAT_DATABASE_URL is required")
	}
	return cfg, nil
}

func LoadClient() Client {
	return Client{
		ServerURL:      getEnv("CHAT_SERVER_URL", "http://localhost:8080"),
		Workspace:      getEnv("CHAT_WORKSPACE", "acme"),
		DefaultHandle:  getEnv("CHAT_HANDLE", ""),
		DefaultChannel: getEnv("CHAT_DEFAULT_CHANNEL", "lobby"),
		LogFormat:      getEnv("CHAT_LOG_FORMAT", "text"),
		ReconnectDelay: getEnvDuration("CHAT_RECONNECT_DELAY", 3*time.Second),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
