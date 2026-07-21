// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ControllerInfo is the subset of GET /api/info we need. omadacId prefixes
// every subsequent /api/v2 path.
type ControllerInfo struct {
	OmadacID      string `json:"omadacId"`
	ControllerVer string `json:"controllerVer"`
	APIVer        string `json:"apiVer"`
	Type          int    `json:"type"` // 0 = software, 1 = OC200, 2 = OC300, ...
}

// loginResult is the payload of a successful login: the CSRF token.
type loginResult struct {
	Token string `json:"token"`
}

// info fetches controller metadata (no auth required).
func (c *Client) info(ctx context.Context) (*ControllerInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/info", nil)
	if err != nil {
		return nil, fmt.Errorf("building info request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading info response: %w", err)
	}

	var env APIResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decoding info envelope (http %d): %w", resp.StatusCode, err)
	}
	if env.ErrorCode != 0 {
		return nil, &APIError{Code: env.ErrorCode, Msg: env.Msg}
	}

	var out ControllerInfo
	if err := json.Unmarshal(env.Result, &out); err != nil {
		return nil, fmt.Errorf("decoding controller info: %w", err)
	}
	if out.OmadacID == "" {
		return nil, fmt.Errorf("controller info returned an empty omadacId")
	}
	return &out, nil
}

// login performs the info + login handshake and stores omadacId + CSRF token.
// It holds the login mutex so concurrent callers don't stampede the controller.
func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	inf, err := c.info(ctx)
	if err != nil {
		return err
	}
	c.omadacID = inf.OmadacID

	endpoint := fmt.Sprintf("%s/%s/api/v2/login", c.baseURL, c.omadacID)
	body, err := json.Marshal(map[string]string{
		"username": c.username,
		"password": c.password,
	})
	if err != nil {
		return fmt.Errorf("marshalling login body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("building login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading login response: %w", err)
	}

	var env APIResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decoding login envelope (http %d): %w", resp.StatusCode, err)
	}
	if env.ErrorCode != 0 {
		return fmt.Errorf("login failed: %w", &APIError{Code: env.ErrorCode, Msg: env.Msg})
	}

	var lr loginResult
	if err := json.Unmarshal(env.Result, &lr); err != nil {
		return fmt.Errorf("decoding login result: %w", err)
	}
	if lr.Token == "" {
		return fmt.Errorf("login succeeded but returned an empty token")
	}
	c.token = lr.Token
	return nil
}

// ControllerVersion returns cached controller metadata (fetched at login).
func (c *Client) ControllerInfo(ctx context.Context) (*ControllerInfo, error) {
	return c.info(ctx)
}
