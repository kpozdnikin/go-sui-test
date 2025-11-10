package config

import (
	"fmt"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type (
	Config struct {
		App        `yaml:"app" env-prefix:"APP_"`
		GRPC       `yaml:"grpc" env-prefix:"GRPC_"`
		HTTP       `yaml:"http" env-prefix:"HTTP_"`
		PostgreSQL `yaml:"postgresql" env-prefix:"POSTGRESQL_"`
		Sync       `yaml:"sync" env-prefix:"SYNC_"`
	}

	App struct {
		Name    string `yaml:"name" env:"NAME"`
		Version string `yaml:"version" env:"VERSION"`
	}

	GRPC struct {
		Port             string `yaml:"port" env:"PORT"`
		EnableReflection bool   `yaml:"enable_reflection" env:"ENABLE_REFLECTION"`
	}

	HTTP struct {
		Port string `yaml:"port" env:"HTTP_PORT"`
	}

	PostgreSQL struct {
		Host     string `yaml:"host" env:"HOST"`
		Port     string `yaml:"port" env:"PORT"`
		User     string `yaml:"user" env:"USER"`
		Password string `yaml:"password" env:"PASSWORD"`
		DBName   string `yaml:"dbname" env:"DBNAME"`
		SSLMode  string `yaml:"sslmode" env:"SSLMODE"`
	}

	Sync struct {
		Interval  time.Duration `yaml:"interval" env:"INTERVAL"`
		BatchSize int           `yaml:"batch_size" env:"BATCH_SIZE"`
	}
)

var (
	instance *Config
	once     sync.Once
)

// GetConfig reads config from file or environment variables
func GetConfig(path string) (*Config, error) {
	var err error

	once.Do(func() {
		instance = &Config{}

		if err = cleanenv.ReadConfig(path, instance); err != nil {
			err = fmt.Errorf("config error: %w", err)
			return
		}
	})

	if err != nil {
		return nil, err
	}

	return instance, nil
}
