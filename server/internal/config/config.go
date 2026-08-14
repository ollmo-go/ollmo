package config

import (
	"fmt"
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
	return cfg, nil
}

// validate fails fast on insecure defaults when running in production.
func (c *Config) validate() error {
	if c.Server.Env != "production" {
		return nil
	}
	if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "change_me" || c.Auth.JWTSecret == "change_me_to_a_long_random_string_in_production" {
		return fmt.Errorf("JWT_SECRET must be set to a secure value in production")
	}
	if c.MySQL.Password == "ollmo_dev_pwd" {
		return fmt.Errorf("MYSQL_PASSWORD must be changed from dev default in production")
	}
	if c.MinIO.SecretKey == "minioadmin" {
		return fmt.Errorf("MINIO_SECRET_KEY must be changed from dev default in production")
	}
	return nil
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
	if v := g("JWT_EXPIRE_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Auth.JWTExpireHours = n
		}
	}
}
