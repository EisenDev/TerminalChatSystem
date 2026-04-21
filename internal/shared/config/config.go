package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	PublicBaseURL  string
	R2Endpoint     string
	R2AccessKey    string
	R2SecretKey    string
	R2Bucket       string
	R2PublicBase   string
	MediaMaxBytes  int64
}

type Client struct {
	ServerURL      string
	Workspace      string
	WorkspaceCode  string
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
		PublicBaseURL:  getEnv("CHAT_PUBLIC_BASE_URL", "https://termichat.zeraynce.com"),
		R2Endpoint:     os.Getenv("CHAT_R2_ENDPOINT"),
		R2AccessKey:    os.Getenv("CHAT_R2_ACCESS_KEY"),
		R2SecretKey:    os.Getenv("CHAT_R2_SECRET_KEY"),
		R2Bucket:       os.Getenv("CHAT_R2_BUCKET"),
		R2PublicBase:   os.Getenv("CHAT_R2_PUBLIC_BASE"),
		MediaMaxBytes:  int64(getEnvInt("CHAT_MEDIA_MAX_BYTES", 25*1024*1024)),
	}
	if cfg.DatabaseURL == "" {
		return Server{}, fmt.Errorf("CHAT_DATABASE_URL is required")
	}
	return cfg, nil
}

func LoadClient() Client {
	fileEnv := loadClientFileEnv()
	return Client{
		ServerURL:      getClientEnv(fileEnv, "CHAT_SERVER_URL", "http://localhost:8080"),
		Workspace:      getClientEnv(fileEnv, "CHAT_WORKSPACE", "acme"),
		WorkspaceCode:  getClientEnv(fileEnv, "CHAT_WORKSPACE_CODE", ""),
		DefaultHandle:  getClientEnv(fileEnv, "CHAT_HANDLE", ""),
		DefaultChannel: getClientEnv(fileEnv, "CHAT_DEFAULT_CHANNEL", "lobby"),
		LogFormat:      getClientEnv(fileEnv, "CHAT_LOG_FORMAT", "text"),
		ReconnectDelay: getClientEnvDuration(fileEnv, "CHAT_RECONNECT_DELAY", 3*time.Second),
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

func getClientEnv(fileEnv map[string]string, key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if v := fileEnv[key]; v != "" {
		return v
	}
	return fallback
}

func getClientEnvDuration(fileEnv map[string]string, key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	if v := fileEnv[key]; v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func loadClientFileEnv() map[string]string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return map[string]string{}
	}
	path := filepath.Join(cfgDir, "teamchat", "client.env")
	file, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return env
}
