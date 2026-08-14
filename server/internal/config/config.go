package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	MySQL  MySQLConfig  `yaml:"mysql"`
	Redis  RedisConfig  `yaml:"redis"`
	MinIO  MinIOConfig  `yaml:"minio"`
	Milvus MilvusConfig `yaml:"milvus"`
	MinerU MinerUConfig `yaml:"mineru"`
	Auth   AuthConfig   `yaml:"auth"`
	Quota  QuotaConfig  `yaml:"quota"`
	Worker WorkerConfig `yaml:"worker"`
}

type ServerConfig struct {
	Port        string `yaml:"port"`
	Env         string `yaml:"env"`
	CORSOrigins string `yaml:"cors_origins"`
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

func (m MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		m.User, m.Password, m.Host, m.Port, m.Database)
}

type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	Password string `yaml:"password"`
}

func (r RedisConfig) Addr() string { return r.Host + ":" + r.Port }

type MinIOConfig struct {
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	UseSSL    bool   `yaml:"use_ssl"`
	Bucket    string `yaml:"bucket"`
}

type MilvusConfig struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

func (m MilvusConfig) Addr() string { return m.Host + ":" + m.Port }

type MinerUConfig struct {
	Endpoint string `yaml:"endpoint"`
}

type AuthConfig struct {
	JWTSecret      string `yaml:"jwt_secret"`
	JWTExpireHours int    `yaml:"jwt_expire_hours"`
	// EncryptionKey is the master key for credential encryption (LLM
	// provider API keys). Independent from JWTSecret so operators can rotate
	// the signing secret without making stored ciphertexts undecryptable.
	// Resolved in Load: falls back to the ORIGINAL JWTSecret when unset.
	EncryptionKey string `yaml:"encryption_key"`
}

// WorkerConfig sizes the two Asynq worker pools. Pipeline slots run the
// document pipeline (parse/embed/extract, including 30-minute MinerU jobs);
// aux slots run short interactive tasks (conversation summaries). Separate
// pools keep a batch of long parses from starving summary generation and
// vice versa. Values <= 0 fall back to the defaults in Load.
type WorkerConfig struct {
	PipelineConcurrency int `yaml:"pipeline_concurrency"`
	AuxConcurrency      int `yaml:"aux_concurrency"`
}

// QuotaConfig holds per-plan resource limits. Plans are matched by name
// (case-insensitive). An unknown plan falls back to Free.
type QuotaConfig struct {
	Free       PlanQuota `yaml:"free"`
	Pro        PlanQuota `yaml:"pro"`
	Enterprise PlanQuota `yaml:"enterprise"`
}

type PlanQuota struct {
	DocQuota         int `yaml:"doc_quota"`
	VectorQuota      int `yaml:"vector_quota"`
	MessageQuota     int `yaml:"message_quota"`
	UserMessageQuota int `yaml:"user_message_quota"`
}

// QuotasFor returns the plan limits for the given plan name. Unknown plans
// fall back to Free so registration never fails on a typo.
func (q QuotaConfig) QuotasFor(plan string) PlanQuota {
	switch normalizePlan(plan) {
	case "pro":
		return q.Pro
	case "enterprise":
		return q.Enterprise
	default:
		return q.Free
	}
}

func normalizePlan(p string) string {
	s := strings.ToLower(strings.TrimSpace(p))
	if s == "" {
		return "free"
	}
	return s
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	overrideFromEnv(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg.resolveSecrets()
	return cfg, nil
}

// weakSecrets are publicly known values shipped in defaults and .env.example.
// They must never sign tokens or encrypt credentials in production, and in
// other environments they are replaced at startup (see resolveSecrets).
func weakSecret(s string) bool {
	switch s {
	case "", "change_me", "change_me_to_a_long_random_string_in_production":
		return true
	}
	return false
}

// validate fails fast on insecure defaults when running in production. The
// default env is development, so every weak value here is also guarded at
// runtime by resolveSecrets for non-production deployments.
func (c *Config) validate() error {
	if c.Server.Env != "production" {
		return nil
	}
	if weakSecret(c.Auth.JWTSecret) {
		return fmt.Errorf("JWT_SECRET must be set to a secure value in production")
	}
	if weakSecret(c.Auth.EncryptionKey) {
		return fmt.Errorf("ENCRYPTION_KEY must be set to a secure value in production (independent from JWT_SECRET)")
	}
	if c.MySQL.Password == "ollmo_dev_pwd" {
		return fmt.Errorf("MYSQL_PASSWORD must be changed from dev default in production")
	}
	if c.MinIO.SecretKey == "minioadmin" {
		return fmt.Errorf("MINIO_SECRET_KEY must be changed from dev default in production")
	}
	return nil
}

// resolveSecrets decouples the JWT signing secret from the credential
// encryption key:
//
//  1. When ENCRYPTION_KEY is unset it falls back to the ORIGINAL JWT secret
//     (captured before any randomization) so existing deployments keep
//     decrypting stored credentials and server/worker containers agree.
//  2. Outside production, a weak (empty or publicly known) JWT secret is
//     replaced with a random per-boot key so tokens are never signed with a
//     publicly known constant. Sessions do not survive restarts until the
//     operator sets JWT_SECRET.
func (c *Config) resolveSecrets() {
	if c.Auth.EncryptionKey == "" {
		c.Auth.EncryptionKey = c.Auth.JWTSecret
		log.Printf("[config] ENCRYPTION_KEY not set; using JWT-secret-derived key for credential encryption. " +
			"Set a dedicated ENCRYPTION_KEY so JWT_SECRET rotation does not affect stored credentials")
	}
	if c.Server.Env != "production" && weakSecret(c.Auth.JWTSecret) {
		log.Printf("[config] WARNING: JWT_SECRET is empty or a publicly known default; " +
			"using a random per-boot signing secret (sessions will not survive restarts). Set JWT_SECRET for stable sessions")
		c.Auth.JWTSecret = randomHex(32)
	}
}

// randomHex returns n cryptographically random bytes as a hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is unrecoverable; a fixed value here would be a
		// publicly known key, which is exactly what this path avoids.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: "8080", Env: "development", CORSOrigins: "http://localhost:3001, http://localhost:3000"},
		MySQL:  MySQLConfig{Host: "localhost", Port: "3306", User: "ollmo", Password: "ollmo_dev_pwd", Database: "ollmo"},
		Redis:  RedisConfig{Host: "localhost", Port: "6379"},
		MinIO:  MinIOConfig{Endpoint: "localhost:9000", AccessKey: "minioadmin", SecretKey: "minioadmin", Bucket: "ollmo"},
		Milvus: MilvusConfig{Host: "localhost", Port: "19530"},
		MinerU: MinerUConfig{Endpoint: "http://localhost:8000"},
		Auth:   AuthConfig{JWTSecret: "change_me", JWTExpireHours: 168},
		Worker: WorkerConfig{PipelineConcurrency: 5, AuxConcurrency: 2},
		Quota: QuotaConfig{
			Free:       PlanQuota{DocQuota: 100, VectorQuota: 10000, MessageQuota: 100, UserMessageQuota: 20},
			Pro:        PlanQuota{DocQuota: 1000, VectorQuota: 100000, MessageQuota: 1000, UserMessageQuota: 100},
			Enterprise: PlanQuota{DocQuota: 10000, VectorQuota: 1000000, MessageQuota: -1, UserMessageQuota: -1},
		},
	}
}

func overrideFromEnv(cfg *Config) {
	g := func(k string) string { return os.Getenv(k) }
	if v := g("SERVER_PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := g("SERVER_ENV"); v != "" {
		cfg.Server.Env = v
	}
	if v := g("CORS_ORIGINS"); v != "" {
		cfg.Server.CORSOrigins = v
	}
	if v := g("MYSQL_HOST"); v != "" {
		cfg.MySQL.Host = v
	}
	if v := g("MYSQL_PORT"); v != "" {
		cfg.MySQL.Port = v
	}
	if v := g("MYSQL_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := g("MYSQL_PASSWORD"); v != "" {
		cfg.MySQL.Password = v
	}
	if v := g("MYSQL_DATABASE"); v != "" {
		cfg.MySQL.Database = v
	}
	if v := g("REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := g("REDIS_PORT"); v != "" {
		cfg.Redis.Port = v
	}
	if v := g("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := g("MINIO_ENDPOINT"); v != "" {
		cfg.MinIO.Endpoint = v
	}
	if v := g("MINIO_ACCESS_KEY"); v != "" {
		cfg.MinIO.AccessKey = v
	}
	if v := g("MINIO_SECRET_KEY"); v != "" {
		cfg.MinIO.SecretKey = v
	}
	if v := g("MINIO_BUCKET"); v != "" {
		cfg.MinIO.Bucket = v
	}
	if v := g("MILVUS_HOST"); v != "" {
		cfg.Milvus.Host = v
	}
	if v := g("MILVUS_PORT"); v != "" {
		cfg.Milvus.Port = v
	}
	if v := g("MINERU_ENDPOINT"); v != "" {
		cfg.MinerU.Endpoint = v
	}
	if v := g("JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := g("ENCRYPTION_KEY"); v != "" {
		cfg.Auth.EncryptionKey = v
	}
	if v := g("JWT_EXPIRE_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Auth.JWTExpireHours = n
		}
	}
	if v := g("WORKER_PIPELINE_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Worker.PipelineConcurrency = n
		}
	}
	if v := g("WORKER_AUX_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Worker.AuxConcurrency = n
		}
	}
	// Guard against yaml/env values that would disable a pool entirely.
	if cfg.Worker.PipelineConcurrency <= 0 {
		cfg.Worker.PipelineConcurrency = 5
	}
	if cfg.Worker.AuxConcurrency <= 0 {
		cfg.Worker.AuxConcurrency = 2
	}
}
