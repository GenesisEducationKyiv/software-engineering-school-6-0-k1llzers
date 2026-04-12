package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultPath = "config.yaml"

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	GitHub   GitHubConfig   `yaml:"github"`
	Mail     MailConfig     `yaml:"mail"`
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

type MailConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	From       string `yaml:"from"`
	ApiBaseUrl string `yaml:"api_base_url"`
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
		Mail: MailConfig{
			Port: 587,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = defaultPath
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

	if cfg.Mail.Port == 0 {
		cfg.Mail.Port = defaults.Mail.Port
	}
}
