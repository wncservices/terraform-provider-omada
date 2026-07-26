// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// openAPIMock emulates the controller's Open API: a client-credentials token
// endpoint plus one protected path that rejects anything but the current token.
type openAPIMock struct {
	srv        *httptest.Server
	tokenCalls int32
	// expired makes the protected path reject the token once, as the
	// controller does when one lapses mid-apply.
	expired atomic.Bool
	issued  atomic.Value // string
}

func newOpenAPIMock(t *testing.T) *openAPIMock {
	t.Helper()
	m := &openAPIMock{}
	m.issued.Store("")
	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		writeEnv(w, 0, "", map[string]any{"omadacId": "cid-1", "controllerVer": "6.0.0"})
	})
	mux.HandleFunc("/cid-1/api/v2/login", func(w http.ResponseWriter, _ *http.Request) {
		writeEnv(w, 0, "", map[string]any{"token": "csrf-1"})
	})

	mux.HandleFunc("/openapi/authorize/token", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["client_id"] != "id-ok" || in["client_secret"] != "secret-ok" {
			writeEnv(w, -44106, "The Client Id Or Client Secret is Invalid.", nil)
			return
		}
		n := atomic.AddInt32(&m.tokenCalls, 1)
		tok := fmt.Sprintf("token-%d", n)
		m.issued.Store(tok)
		writeEnv(w, 0, "", map[string]any{"accessToken": tok, "tokenType": "Bearer", "expiresIn": 7200})
	})

	mux.HandleFunc("/openapi/v1/cid-1/sites/site-1/thing", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		want := "AccessToken=" + m.issued.Load().(string)
		if m.expired.Load() {
			// One-shot: the next call gets a fresh token and must succeed.
			m.expired.Store(false)
			writeEnv(w, -44112, "token expired", nil)
			return
		}
		if auth != want {
			writeEnv(w, -44116, "Open API Authorized failed", nil)
			return
		}
		writeEnv(w, 0, "", map[string]any{"ok": true})
	})

	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	return m
}

func writeEnv(w http.ResponseWriter, code int, msg string, result any) {
	var raw json.RawMessage
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": code, "msg": msg, "result": raw})
}

func newTestClient(t *testing.T, url, id, secret string) *Client {
	t.Helper()
	c, err := NewClientWithOpenAPI(context.Background(), url, "admin", "pw", id, secret, true)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return c
}

// TestOpenAPINotConfigured checks the error a practitioner hits first is the
// actionable one, not a bare authorisation failure.
func TestOpenAPINotConfigured(t *testing.T) {
	m := newOpenAPIMock(t)
	c := newTestClient(t, m.srv.URL, "", "")

	err := c.DoOpenAPI(context.Background(), http.MethodGet, c.OpenAPIPath("site-1", "/thing"), nil, nil)
	if err == nil {
		t.Fatal("expected an error when no Open API credentials are configured")
	}
	for _, want := range []string{"Platform Integration", "openapi_client_id", "-44116"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
	if atomic.LoadInt32(&m.tokenCalls) != 0 {
		t.Error("should not have called the token endpoint without credentials")
	}
}

// TestOpenAPITokenIsCached checks a second call reuses the token rather than
// re-authenticating on every request.
func TestOpenAPITokenIsCached(t *testing.T) {
	m := newOpenAPIMock(t)
	c := newTestClient(t, m.srv.URL, "id-ok", "secret-ok")
	ctx := context.Background()
	path := c.OpenAPIPath("site-1", "/thing")

	for i := 0; i < 3; i++ {
		if err := c.DoOpenAPI(ctx, http.MethodGet, path, nil, nil); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&m.tokenCalls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1 (the token should be cached)", got)
	}
}

// TestOpenAPIRetriesOnExpiredToken checks a lapsed token is refreshed and the
// request retried once, rather than surfacing as a failed apply.
func TestOpenAPIRetriesOnExpiredToken(t *testing.T) {
	m := newOpenAPIMock(t)
	c := newTestClient(t, m.srv.URL, "id-ok", "secret-ok")
	ctx := context.Background()
	path := c.OpenAPIPath("site-1", "/thing")

	if err := c.DoOpenAPI(ctx, http.MethodGet, path, nil, nil); err != nil {
		t.Fatalf("first call: %v", err)
	}
	m.expired.Store(true)
	if err := c.DoOpenAPI(ctx, http.MethodGet, path, nil, nil); err != nil {
		t.Fatalf("call after expiry should have retried with a fresh token: %v", err)
	}
	if got := atomic.LoadInt32(&m.tokenCalls); got != 2 {
		t.Errorf("token endpoint called %d times, want 2 (initial + refresh)", got)
	}
}

// TestOpenAPIBadCredentials checks the controller's -44106 surfaces intact,
// since that is what a wrong or deleted application looks like.
func TestOpenAPIBadCredentials(t *testing.T) {
	m := newOpenAPIMock(t)
	c := newTestClient(t, m.srv.URL, "id-wrong", "secret-wrong")

	err := c.DoOpenAPI(context.Background(), http.MethodGet, c.OpenAPIPath("site-1", "/thing"), nil, nil)
	if err == nil {
		t.Fatal("expected an error with bad credentials")
	}
	if !strings.Contains(err.Error(), "-44106") {
		t.Errorf("error should carry the controller code -44106, got: %v", err)
	}
}
