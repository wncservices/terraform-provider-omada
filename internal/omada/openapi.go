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
	"sync"
	"time"
)

// The controller exposes a second API alongside the web one: TP-Link's
// documented **Open API**, served under /openapi/. Two capabilities live only
// there — creating a network, and per-device configuration such as switch-port
// settings — so a provider that wants either has to speak it.
//
// It does not accept the web session. A request carrying a valid Csrf-Token and
// cookie is refused with -44116 ("Open API Authorized failed"); the controller
// UI only gets away with it because its requests are proxied through TP-Link's
// cloud connector, which authenticates on the operator's behalf. Locally the
// only way in is a client-credentials grant:
//
//	POST /openapi/authorize/token?grant_type=client_credentials
//	{"omadacId": …, "client_id": …, "client_secret": …}
//	-> {"result": {"accessToken": …, "expiresIn": 7200, …}}
//
// and then `Authorization: AccessToken=<token>` on every call.
//
// The credentials come from an application registered in the controller under
// Settings -> Platform Integration -> Open API. They are separate from the
// admin username/password and are configured separately on the provider.

// openAPITokenPath is the client-credentials grant endpoint. Note it is not
// prefixed with the omadacId, unlike every /api/v2 path.
const openAPITokenPath = "/openapi/authorize/token?grant_type=client_credentials"

// openAPIExpirySkew renews a token slightly before it actually expires, so a
// long apply cannot have one expire mid-flight.
const openAPIExpirySkew = 60 * time.Second

// openAPIAuth holds the client-credentials and the cached access token.
type openAPIAuth struct {
	clientID     string
	clientSecret string

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// openAPIConfigured reports whether Open API credentials were supplied.
func (c *Client) openAPIConfigured() bool {
	return c.openAPI != nil && c.openAPI.clientID != "" && c.openAPI.clientSecret != ""
}

// ErrOpenAPINotConfigured is returned by any operation that needs the Open API
// when no credentials were given. The message is deliberately actionable: this
// is the error a practitioner meets first, and "unauthorized" would send them
// looking at the wrong credentials entirely.
var ErrOpenAPINotConfigured = fmt.Errorf(
	"this operation uses the controller's Open API, which needs its own credentials.\n" +
		"Register an application under Settings -> Platform Integration -> Open API, then set\n" +
		"`openapi_client_id` and `openapi_client_secret` on the provider (or OMADA_OPENAPI_CLIENT_ID\n" +
		"and OMADA_OPENAPI_CLIENT_SECRET). The admin username and password do not grant Open API\n" +
		"access: the controller refuses a web session there with error -44116")

// openAPITokenResult is the grant response.
type openAPITokenResult struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
}

// openAPIToken returns a valid access token, fetching one if the cache is empty
// or close to expiry.
func (c *Client) openAPIToken(ctx context.Context, force bool) (string, error) {
	if !c.openAPIConfigured() {
		return "", ErrOpenAPINotConfigured
	}
	a := c.openAPI
	a.mu.Lock()
	defer a.mu.Unlock()

	if !force && a.token != "" && time.Now().Before(a.expiry) {
		return a.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"omadacId":      c.omadacID,
		"client_id":     a.clientID,
		"client_secret": a.clientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("marshalling open api token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+openAPITokenPath, strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("building open api token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting open api token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading open api token response: %w", err)
	}

	var env APIResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("decoding open api token envelope (http %d): %w", resp.StatusCode, err)
	}
	if env.ErrorCode != 0 {
		// -44106 is the one people actually hit: the id or secret is wrong, or
		// the application was removed from the controller.
		return "", fmt.Errorf("open api token request rejected: %w", &APIError{Code: env.ErrorCode, Msg: env.Msg})
	}

	var out openAPITokenResult
	if err := json.Unmarshal(env.Result, &out); err != nil {
		return "", fmt.Errorf("decoding open api token: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("open api token request succeeded but returned no access token")
	}

	a.token = out.AccessToken
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl > openAPIExpirySkew {
		ttl -= openAPIExpirySkew
	}
	a.expiry = time.Now().Add(ttl)
	return a.token, nil
}

// isOpenAPIAuthError reports controller codes meaning the access token is no
// longer good, so the caller should get a fresh one and retry once.
func isOpenAPIAuthError(code int) bool {
	switch code {
	case -44112, // token expired
		-44113, // token invalid
		-44116: // authorisation failed
		return true
	default:
		return false
	}
}

// OpenAPIPath builds a site-scoped Open API path on v1.
func (c *Client) OpenAPIPath(siteID, suffix string) string {
	return c.OpenAPIPathVersion(1, siteID, suffix)
}

// OpenAPIPathVersion builds a site-scoped Open API path on a given version.
//
// The version is per-endpoint, not global: `devices` and `switches/{mac}/ports`
// are v1 while `lan-networks` and `lan-profiles` are v2. There is no prefix that
// works for everything, so each caller states the one its endpoint uses.
func (c *Client) OpenAPIPathVersion(version int, siteID, suffix string) string {
	return fmt.Sprintf("/openapi/v%d/%s/sites/%s%s", version, c.omadacID, siteID, suffix)
}

// DoOpenAPI performs an authenticated Open API request, unmarshalling result
// into out. Like Do, it retries once after refreshing credentials that the
// controller has stopped accepting.
func (c *Client) DoOpenAPI(ctx context.Context, method, path string, body, out any) error {
	return c.doOpenAPI(ctx, method, path, body, out, true)
}

func (c *Client) doOpenAPI(ctx context.Context, method, path string, body, out any, retry bool) error {
	token, err := c.openAPIToken(ctx, false)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling request body: %w", err)
		}
		reader = strings.NewReader(string(buf))
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building open api request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "AccessToken="+token)

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
		return fmt.Errorf("decoding open api envelope (%s %s, http %d): %w", method, path, resp.StatusCode, err)
	}

	if isOpenAPIAuthError(env.ErrorCode) && retry {
		if _, err := c.openAPIToken(ctx, true); err != nil {
			return fmt.Errorf("refreshing open api token: %w", err)
		}
		return c.doOpenAPI(ctx, method, path, body, out, false)
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
