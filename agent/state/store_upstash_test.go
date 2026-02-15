package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpstashRedisStoreRedisKey(t *testing.T) {
	t.Parallel()

	store := &UpstashRedisStore{}
	got, err := store.redisKey("abc")
	if err != nil {
		t.Fatalf("redisKey() error = %v", err)
	}
	if got != "conv:abc:agent:session" {
		t.Fatalf("redisKey() = %q, want %q", got, "conv:abc:agent:session")
	}
}

func TestUpstashRedisStoreRedisKeyEmptySession(t *testing.T) {
	t.Parallel()

	store := &UpstashRedisStore{}
	_, err := store.redisKey("   ")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("redisKey() error = %v, want ErrInvalidSession", err)
	}
}

func TestUpstashRedisStoreSaveUsesHardcodedSessionKey(t *testing.T) {
	t.Parallel()

	const wantKey = "conv:session-1:agent:session"
	var gotCommand []any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotCommand); err != nil {
			t.Fatalf("decode command: %v", err)
		}
		fmt.Fprint(w, `{"result":"OK"}`)
	}))
	t.Cleanup(server.Close)

	store, err := NewUpstashRedisStore(
		UpstashRedisConfig{
			URL:   server.URL,
			Token: "token",
		},
		WithHTTPClient(server.Client()),
		WithTTL(0),
	)
	if err != nil {
		t.Fatalf("NewUpstashRedisStore() error = %v", err)
	}

	state := NewSessionState("session-1", "ws", "cust", "chat", time.Now().UTC())
	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if len(gotCommand) < 2 {
		t.Fatalf("unexpected command: %#v", gotCommand)
	}
	if gotCommand[0] != "SET" {
		t.Fatalf("command[0] = %v, want SET", gotCommand[0])
	}
	if gotCommand[1] != wantKey {
		t.Fatalf("command[1] = %v, want %s", gotCommand[1], wantKey)
	}
}

func TestUpstashRedisStoreLoadUsesHardcodedSessionKey(t *testing.T) {
	t.Parallel()

	const wantKey = "conv:session-2:agent:session"
	var gotCommand []any

	seed := NewSessionState("session-2", "ws", "cust", "chat", time.Now().UTC())
	payload, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	encoded, err := json.Marshal(string(payload))
	if err != nil {
		t.Fatalf("marshal encoded seed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotCommand); err != nil {
			t.Fatalf("decode command: %v", err)
		}
		fmt.Fprintf(w, `{"result":%s}`, encoded)
	}))
	t.Cleanup(server.Close)

	store, err := NewUpstashRedisStore(
		UpstashRedisConfig{
			URL:   server.URL,
			Token: "token",
		},
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("NewUpstashRedisStore() error = %v", err)
	}

	st, err := store.Load(context.Background(), "session-2")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if st.SessionID != "session-2" {
		t.Fatalf("Load().SessionID = %q, want %q", st.SessionID, "session-2")
	}

	if len(gotCommand) < 2 {
		t.Fatalf("unexpected command: %#v", gotCommand)
	}
	if gotCommand[0] != "GET" {
		t.Fatalf("command[0] = %v, want GET", gotCommand[0])
	}
	if gotCommand[1] != wantKey {
		t.Fatalf("command[1] = %v, want %s", gotCommand[1], wantKey)
	}
}

func TestUpstashRedisStoreDeleteUsesHardcodedSessionKey(t *testing.T) {
	t.Parallel()

	const wantKey = "conv:session-3:agent:session"
	var gotCommand []any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotCommand); err != nil {
			t.Fatalf("decode command: %v", err)
		}
		fmt.Fprint(w, `{"result":1}`)
	}))
	t.Cleanup(server.Close)

	store, err := NewUpstashRedisStore(
		UpstashRedisConfig{
			URL:   server.URL,
			Token: "token",
		},
		WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("NewUpstashRedisStore() error = %v", err)
	}

	if err := store.Delete(context.Background(), "session-3"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if len(gotCommand) < 2 {
		t.Fatalf("unexpected command: %#v", gotCommand)
	}
	if gotCommand[0] != "DEL" {
		t.Fatalf("command[0] = %v, want DEL", gotCommand[0])
	}
	if gotCommand[1] != wantKey {
		t.Fatalf("command[1] = %v, want %s", gotCommand[1], wantKey)
	}
}
