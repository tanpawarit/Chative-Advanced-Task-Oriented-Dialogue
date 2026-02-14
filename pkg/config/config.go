package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/viper"
)

var (
	envFilePath string
	parseOnce   sync.Once
)

func MustNew[T any](prefix string) *T {
	conf, err := New[T](prefix)
	if err != nil {
		panic(err)
	}
	return conf
}

func New[T any](prefix string) (*T, error) {
	filepath := resolveEnvPath()
	if filepath != "" {
		if err := exportEnvironment(filepath); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	} else if err := exportEnvironmentIfExists(".env"); err != nil {
		return nil, fmt.Errorf("failed to load default env file: %w", err)
	}

	var conf T
	if err := envconfig.Process(prefix, &conf); err != nil {
		return nil, err
	}

	return &conf, nil
}

func resolveEnvPath() string {
	parseOnce.Do(func() {
		if flag.Lookup("env") == nil {
			flag.StringVar(&envFilePath, "env", "", "path to .env file")
		}
		if !flag.Parsed() {
			flag.Parse()
		}
	})
	return strings.TrimSpace(envFilePath)
}

func exportEnvironmentIfExists(filepath string) error {
	info, err := os.Stat(filepath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	return exportEnvironment(filepath)
}

func exportEnvironment(filepath string) error {
	viper.SetConfigFile(filepath)
	if err := viper.ReadInConfig(); err != nil {
		return err
	}

	for k, v := range viper.AllSettings() {
		if err := os.Setenv(strings.ToUpper(k), fmt.Sprint(v)); err != nil {
			return err
		}
	}

	return nil
}
