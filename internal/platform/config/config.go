package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	appDefaultPath          = "app-config.yaml"
	notificationDefaultPath = "notification-config.yaml"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	GitHub   GitHubConfig   `yaml:"github"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	Mail     MailConfig     `yaml:"mail"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

type GitHubConfig struct {
	Token string `yaml:"token"`
}

type RabbitMQConfig struct {
	URL                  string `yaml:"url"`
	NotificationExchange string `yaml:"notification_exchange"`
	NotificationQueue    string `yaml:"notification_queue"`
}

type MailConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	ApiBaseUrl string `yaml:"api_base_url"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Port: "8080",
		},
		Database: DatabaseConfig{
			URL: "postgres://postgres:postgres@localhost:5432/github_release_notifier?sslmode=disable",
		},
		GitHub: GitHubConfig{
			Token: "",
		},
		RabbitMQ: RabbitMQConfig{
			URL:                  "",
			NotificationExchange: "notifications",
			NotificationQueue:    "notification-service",
		},
		Mail: MailConfig{
			Port:       587,
			ApiBaseUrl: "http://localhost:8080/api",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func LoadApp() (Config, error) {
	return Load(appDefaultPath)
}

func LoadNotification() (Config, error) {
	return Load(notificationDefaultPath)
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = appDefaultPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}

		return Config{}, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	defaults := Default()

	if cfg.Server.Port == "" {
		cfg.Server.Port = defaults.Server.Port
	}

	if cfg.Database.URL == "" {
		cfg.Database.URL = defaults.Database.URL
	}

	if cfg.RabbitMQ.NotificationExchange == "" {
		cfg.RabbitMQ.NotificationExchange = defaults.RabbitMQ.NotificationExchange
	}

	if cfg.RabbitMQ.NotificationQueue == "" {
		cfg.RabbitMQ.NotificationQueue = defaults.RabbitMQ.NotificationQueue
	}

	if cfg.Mail.Port == 0 {
		cfg.Mail.Port = defaults.Mail.Port
	}

	if cfg.Mail.ApiBaseUrl == "" {
		cfg.Mail.ApiBaseUrl = defaults.Mail.ApiBaseUrl
	}

	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaults.Logging.Level
	}

	if cfg.Logging.Format == "" {
		cfg.Logging.Format = defaults.Logging.Format
	}
}
