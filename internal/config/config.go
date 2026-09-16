package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the DataAgent server.
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Mongo     MongoConfig     `mapstructure:"mongo"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Qdrant    QdrantConfig    `mapstructure:"qdrant"`
	SeaweedFS SeaweedFSConfig `mapstructure:"seaweedfs"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Log       LogConfig       `mapstructure:"log"`
	Voice     VoiceConfig     `mapstructure:"voice"`
}

type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type MongoConfig struct {
	URI      string `mapstructure:"uri"`
	Database string `mapstructure:"database"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type QdrantConfig struct {
	Addr string `mapstructure:"addr"`
}

type SeaweedFSConfig struct {
	Master string `mapstructure:"master"`
	Filer  string `mapstructure:"filer"`
}

type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Expiration time.Duration `mapstructure:"expiration"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// VoiceConfig configures the server-side speech-to-text integration
// (SPEC-099): the whisper sidecar endpoint and the chunked-upload session
// buffering limits.
type VoiceConfig struct {
	Endpoint      string        `mapstructure:"endpoint"`
	Timeout       time.Duration `mapstructure:"timeout"`
	SessionTTL    time.Duration `mapstructure:"session_ttl"`
	MaxChunkBytes int64         `mapstructure:"max_chunk_bytes"`
}

// Load reads config from file and environment variables.
// Environment variables override file values using Viper's automatic env binding.
// Key env vars: MONGO_URI, REDIS_ADDR, QDRANT_URL, SEAWEEDFS_MASTER, SEAWEEDFS_FILER, JWT_SECRET
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	// Explicit env var bindings for Docker/CI overrides
	_ = v.BindEnv("mongo.uri", "MONGO_URI")
	_ = v.BindEnv("redis.addr", "REDIS_ADDR")
	_ = v.BindEnv("qdrant.addr", "QDRANT_URL")
	_ = v.BindEnv("seaweedfs.master", "SEAWEEDFS_MASTER")
	_ = v.BindEnv("seaweedfs.filer", "SEAWEEDFS_FILER")
	_ = v.BindEnv("jwt.secret", "JWT_SECRET")
	_ = v.BindEnv("server.read_timeout", "SERVER_READ_TIMEOUT")
	_ = v.BindEnv("server.write_timeout", "SERVER_WRITE_TIMEOUT")
	_ = v.BindEnv("voice.endpoint", "VOICE_ENDPOINT")
	_ = v.BindEnv("voice.timeout", "VOICE_TIMEOUT")
	_ = v.BindEnv("voice.session_ttl", "VOICE_SESSION_TTL")
	_ = v.BindEnv("voice.max_chunk_bytes", "VOICE_MAX_CHUNK_BYTES")

	// Set defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "600s")
	v.SetDefault("server.write_timeout", "600s") // SSE streaming + ReAct tool execution
	v.SetDefault("mongo.uri", "mongodb://localhost:27017")
	v.SetDefault("mongo.database", "data_agent")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("qdrant.addr", "localhost:6334")
	v.SetDefault("seaweedfs.master", "http://localhost:9333")
	v.SetDefault("seaweedfs.filer", "http://localhost:8888")
	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.expiration", "24h")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("voice.endpoint", "")      // empty = sidecar unconfigured, finish returns 503
	v.SetDefault("voice.timeout", "60s")    // sidecar transcription timeout
	v.SetDefault("voice.session_ttl", "60s") // recording session idle timeout
	v.SetDefault("voice.max_chunk_bytes", 160000) // 5s × 16kHz × 2B

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Config file not found is OK — use defaults and env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}
