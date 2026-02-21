package logx

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Debug        bool `split_words:"true" default:"false"`
	PrettyFormat bool `split_words:"true" default:"false"`
}

var DefaultConfig = &Config{
	Debug:        false,
	PrettyFormat: false,
}

func safe(opts ...Config) *Config {
	if len(opts) == 0 {
		return DefaultConfig
	}
	return &opts[0]
}

func Init(opts ...Config) {
	conf := safe(opts...)

	if conf.PrettyFormat {
		log.Logger = zerolog.New(zerolog.NewConsoleWriter()).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	if conf.Debug {
		log.Logger = log.Logger.Level(zerolog.DebugLevel)
	} else {
		log.Logger = log.Logger.Level(zerolog.InfoLevel)
	}

	log.Logger = log.Logger.With().Caller().Stack().Logger()
}
