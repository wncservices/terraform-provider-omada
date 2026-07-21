// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

// Package omada is a thin client for the reverse-engineered TP-Link Omada
// controller web API (/api/v2/...), as used by the controller UI itself.
//
// It is verified against Omada v6 software/hardware (OC200/OC300) controllers.
// TP-Link publishes no documentation for this API; endpoint and payload shapes
// were derived from the controller UI. See the package README for references.
package omada

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a stateful client for a single Omada controller. It is safe for
// concurrent use: the login mutex serialises (re-)authentication.
type Client struct {
	baseURL  string
	username string
	password string

	http *http.Client
	mu   sync.Mutex // guards omadacID + token during (re-)login

	omadacID string // controller id, prefixes every /api/v2 path
	token    string // CSRF token, sent as the Csrf-Token header
}

// APIResponse is the standard envelope returned by every controller endpoint.
type APIResponse struct {
	ErrorCode int             `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// PaginatedResult wraps list endpoints that page their data.
type PaginatedResult struct {
	TotalRows   int             `json:"totalRows"`
	CurrentPage int             `json:"currentPage"`
	CurrentSize int             `json:"currentSize"`
	Data        json.RawMessage `json:"data"`
}

// APIError is returned when the controller responds with a non-zero errorCode.
type APIError struct {
	Code int
	Msg  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("omada api error %d: %s", e.Code, e.Msg)
}

// errSessionExpired is the controller errorCode signalling an expired session.
// -1200 (session timeout) and -1400/-1401 (token invalid) are treated as such.
func isSessionExpired(code int) bool {
	switch code {
	case -1200, -1400, -1401:
		return true
	default:
		return false
	}
}

// NewClient builds a client and performs the initial info + login handshake.
func NewClient(ctx context.Context, rawURL, username, password string, skipTLSVerify bool) (*Client, error) {
	base := strings.TrimRight(rawURL, "/")
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("invalid controller url %q: %w", rawURL, err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipTLSVerify}, //nolint:gosec // self-signed controller certs are the norm; gated by skip_tls_verify
	}

	c := &Client{
		baseURL:  base,
		username: username,
		password: password,
		http: &http.Client{
			Jar:       jar,
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}

	if err := c.login(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Do performs an authenticated request against /{omadacId}/api/v2 + path,
// unmarshalling result into out. It transparently re-logs-in once if the
// session has expired.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	if err := c.do(ctx, method, path, body, out, true); err != nil {
		return err
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, retry bool) error {
	c.mu.Lock()
	omadacID, token := c.omadacID, c.token
	c.mu.Unlock()

	endpoint := fmt.Sprintf("%s/%s/api/v2%s", c.baseURL, omadacID, path)

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Csrf-Token", token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var env APIResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decoding response envelope (%s %s, http %d): %w", method, path, resp.StatusCode, err)
	}

	if isSessionExpired(env.ErrorCode) && retry {
		if err := c.login(ctx); err != nil {
			return fmt.Errorf("re-login after expired session: %w", err)
		}
		return c.do(ctx, method, path, body, out, false)
	}

	if env.ErrorCode != 0 {
		return &APIError{Code: env.ErrorCode, Msg: env.Msg}
	}

	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decoding result (%s %s): %w", method, path, err)
		}
	}
	return nil
}
