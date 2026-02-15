package state

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrStateNotFound   = errors.New("session state not found")
	ErrNilSessionState = errors.New("session state is nil")
	ErrInvalidSession  = errors.New("session id is empty")
)

const (
	defaultStoreKeyPrefix = "atod:session:"
	defaultStoreTTL       = 24 * time.Hour
	maxResponseSizeBytes  = 2 << 20
)

// Store is the persistence contract used by the orchestrator.
type Store interface {
	Load(ctx context.Context, sessionID string) (*SessionState, error)
	Save(ctx context.Context, st *SessionState) error
	Delete(ctx context.Context, sessionID string) error
}

// StoreOption customizes UpstashRedisStore.
type StoreOption func(*UpstashRedisStore)

func WithKeyPrefix(prefix string) StoreOption {
	return func(s *UpstashRedisStore) {
		trimmed := strings.TrimSpace(prefix)
		if trimmed != "" {
			s.keyPrefix = trimmed
		}
	}
}

func WithTTL(ttl time.Duration) StoreOption {
	return func(s *UpstashRedisStore) {
		s.ttl = ttl
	}
}

func WithHTTPClient(client *http.Client) StoreOption {
	return func(s *UpstashRedisStore) {
		if client != nil {
			s.httpClient = client
		}
	}
}

// UpstashRedisStore persists SessionState in Upstash Redis via REST.
type UpstashRedisStore struct {
	baseURL    string
	token      string
	httpClient *http.Client
	keyPrefix  string
	ttl        time.Duration
}

type redisRESTResponse struct {
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type UpstashRedisConfig struct {
	URL     string        `envconfig:"URL" split_words:"true" required:"true"`
	Token   string        `envconfig:"TOKEN" split_words:"true" required:"true"`
	Timeout time.Duration `envconfig:"TIMEOUT" split_words:"true" default:"10s"`
}

func NewUpstashRedisStore(cfg UpstashRedisConfig, opts ...StoreOption) (*UpstashRedisStore, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if baseURL == "" {
		return nil, errors.New("upstash redis url is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid redis rest url: %w", err)
	}

	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, errors.New("upstash redis token is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	store := &UpstashRedisStore{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		keyPrefix: defaultStoreKeyPrefix,
		ttl:       defaultStoreTTL,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}

	if store.httpClient == nil {
		store.httpClient = &http.Client{
			Timeout: timeout,
		}
	}
	if store.ttl < 0 {
		return nil, errors.New("ttl must be >= 0")
	}

	return store, nil
}

func (s *UpstashRedisStore) Load(ctx context.Context, sessionID string) (*SessionState, error) {
	key, err := s.redisKey(sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := s.exec(ctx, []any{"GET", key})
	if err != nil {
		return nil, err
	}

	result := bytes.TrimSpace(resp.Result)
	if len(result) == 0 || bytes.Equal(result, []byte("null")) {
		return nil, ErrStateNotFound
	}

	var encoded string
	if err := json.Unmarshal(result, &encoded); err != nil {
		return nil, fmt.Errorf("decode session payload: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal([]byte(encoded), &state); err != nil {
		return nil, fmt.Errorf("unmarshal session state: %w", err)
	}

	state.EnsureGoalsMap()
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("invalid session state loaded from store: %w", err)
	}

	return &state, nil
}

func (s *UpstashRedisStore) Save(ctx context.Context, st *SessionState) error {
	if st == nil {
		return ErrNilSessionState
	}
	if strings.TrimSpace(st.SessionID) == "" {
		return ErrInvalidSession
	}
	if st.Version <= 0 {
		st.Version = 1
	}
	st.EnsureGoalsMap()
	if st.UpdatedAt.IsZero() {
		st.UpdatedAt = time.Now().UTC()
	} else {
		st.UpdatedAt = st.UpdatedAt.UTC()
	}

	key, err := s.redisKey(st.SessionID)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal session state: %w", err)
	}

	cmd := []any{"SET", key, string(payload)}
	if s.ttl > 0 {
		cmd = append(cmd, "EX", ttlSeconds(s.ttl))
	}

	if _, err := s.exec(ctx, cmd); err != nil {
		return err
	}

	return nil
}

func (s *UpstashRedisStore) Delete(ctx context.Context, sessionID string) error {
	key, err := s.redisKey(sessionID)
	if err != nil {
		return err
	}
	_, err = s.exec(ctx, []any{"DEL", key})
	return err
}

func (s *UpstashRedisStore) redisKey(sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", ErrInvalidSession
	}
	prefix := strings.TrimSpace(s.keyPrefix)
	return prefix + sessionID, nil
}

func (s *UpstashRedisStore) exec(ctx context.Context, command []any) (*redisRESTResponse, error) {
	if s == nil {
		return nil, errors.New("nil store")
	}
	if len(command) == 0 {
		return nil, errors.New("empty redis command")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return nil, errors.New("empty redis url")
	}
	if strings.TrimSpace(s.token) == "" {
		return nil, errors.New("empty redis token")
	}

	body, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("marshal redis command: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build redis request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute redis request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSizeBytes))
	if err != nil {
		return nil, fmt.Errorf("read redis response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("redis http status=%d body=%s", resp.StatusCode, string(raw))
	}

	var parsed redisRESTResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode redis response: %w", err)
	}
	if parsed.Error != "" {
		return nil, errors.New(parsed.Error)
	}
	return &parsed, nil
}

func ttlSeconds(ttl time.Duration) int64 {
	seconds := ttl / time.Second
	if seconds <= 0 {
		return 1
	}
	if ttl%time.Second != 0 {
		seconds++
	}
	return int64(seconds)
}
