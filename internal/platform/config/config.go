package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	appDefaultPath          = "app-config.yaml"
	notificationDefaultPath = "notification-config.yaml"
	quotaDefaultPath        = "quota-config.yaml"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	GitHub   GitHubConfig   `yaml:"github"`
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`
	Mail     MailConfig     `yaml:"mail"`
	Quota    QuotaConfig    `yaml:"quota"`
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

type QuotaConfig struct {
	GRPCAddress              string `yaml:"grpc_address"`
	GRPCPort                 string `yaml:"grpc_port"`
	DefaultSubscriptionLimit int    `yaml:"default_subscription_limit"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func LoadApp() (Config, error) {
	return Load(appDefaultPath)
}

func LoadNotification() (Config, error) {
	return Load(notificationDefaultPath)
}

func LoadQuota() (Config, error) {
	return Load(quotaDefaultPath)
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, fmt.Errorf("config path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
