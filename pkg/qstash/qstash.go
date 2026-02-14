package qstash

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	URL               string        `split_words:"true" required:"true"`
	Token             string        `split_words:"true" required:"true"`
	CurrentSigningKey string        `split_words:"true" required:"true"`
	NextSigningKey    string        `split_words:"true" required:"true"`
	Timeout           time.Duration `split_words:"true" default:"10s"`
}

type Client struct {
	baseURL           string
	token             string
	currentSigningKey string
	nextSigningKey    string
	httpClient        *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimSpace(cfg.URL)
	if baseURL == "" {
		return nil, errors.New("qstash url is required")
	}

	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		token:             strings.TrimSpace(cfg.Token),
		currentSigningKey: strings.TrimSpace(cfg.CurrentSigningKey),
		nextSigningKey:    strings.TrimSpace(cfg.NextSigningKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	return client, nil
}

func MustNew(cfg Config) *Client {
	client, err := NewClient(cfg)
	if err != nil {
		panic(err)
	}
	return client
}
