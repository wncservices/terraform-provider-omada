// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mfaController mimics a controller with two-factor authentication enforced:
// the password alone is refused with -30165 plus a challenge, and the session
// only exists once the right code comes back to checkMFACodeAndLogin. That is
// the exchange the controller's own login page performs.
type mfaController struct {
	secret string // base32, what the account was enrolled with

	// mfaID is what the challenge carries and expects echoed back. Empty
	// reproduces a live 6.1.0.19 OC200, whose challenge result is `{}`.
	mfaID string

	// supportedMFATypes is what the challenge advertises. Empty means the
	// field is omitted, which is also what the live controller does.
	supportedMFATypes []int

	// rejectCode forces the "wrong code" answer, for the failure paths.
	rejectCode bool

	// rejectFirstCode refuses whatever code arrives first, which is what a
	// controller does with one another process already spent.
	rejectFirstCode bool

	// remainAttempts, when set, is reported with each rejection and counts
	// down — the controller's own budget before it locks the account.
	remainAttempts *int

	// lockedMinutes, when set, is reported with each rejection: the
	// controller has already locked the account.
	lockedMinutes *int

	loginAttempts int
	codesSeen     []string
}

func (m *mfaController) server(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, 0, "", map[string]any{
			"omadacId": "abc123", "controllerVer": "6.1.0", "apiVer": "3", "type": 1,
		})
	})

	mux.HandleFunc("/abc123/api/v2/login", func(w http.ResponseWriter, _ *http.Request) {
		m.loginAttempts++
		result := map[string]any{}
		if m.mfaID != "" {
			result["MFAId"] = m.mfaID
		}
		if len(m.supportedMFATypes) > 0 {
			result["supportedMFATypes"] = m.supportedMFATypes
		}
		writeEnvelope(w, -30165, "Two-Factor Authentication is required for local user.", result)
	})

	mux.HandleFunc("/abc123/api/v2/checkMFACodeAndLogin", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Code     string `json:"code"`
			MFAId    string `json:"MFAId"`
			MFAType  int    `json:"mfaType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeEnvelope(w, -1001, "bad body", nil)
			return
		}
		m.codesSeen = append(m.codesSeen, body.Code)

		if body.MFAId != m.mfaID || body.MFAType != mfaTypeTOTP || body.Username != "admin" || body.Password != "secret" {
			writeEnvelope(w, -30109, "Invalid username or password.", nil)
			return
		}

		cfg, err := parseTOTPSecret(m.secret)
		if err != nil {
			t.Errorf("fixture secret: %v", err)
		}
		spent := m.rejectFirstCode && len(m.codesSeen) == 1

		if m.rejectCode || spent || body.Code != cfg.codeAt(time.Now()) {
			result := map[string]any{}
			if m.remainAttempts != nil {
				*m.remainAttempts--
				result["codeRemainAttempts"] = *m.remainAttempts
			}
			if m.lockedMinutes != nil {
				result["lockedMinutes"] = *m.lockedMinutes
			}
			writeEnvelope(w, -30139, "Invalid code.", result)
			return
		}
		writeEnvelope(w, 0, "", map[string]any{"token": "tok-2fa", "roleType": 0})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// The live shape: the challenge result is `{}`, and the code alone completes
// the login.
func TestLoginAnswersTOTPChallengeWithoutAnMFAId(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed}
	srv := ctrl.server(t)

	c, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
	if c.token != "tok-2fa" {
		t.Errorf("token = %q, want the one issued after the 2FA step", c.token)
	}
}

func TestLoginAnswersTOTPChallenge(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed, mfaID: "mfa-1", supportedMFATypes: []int{mfaTypeTOTP}}
	srv := ctrl.server(t)

	c, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
	if c.token != "tok-2fa" {
		t.Errorf("token = %q, want the one issued after the 2FA step", c.token)
	}
	if len(ctrl.codesSeen) != 1 {
		t.Errorf("sent %d codes, want exactly 1 — retries burn the account's attempts", len(ctrl.codesSeen))
	}
	if c.lastTOTPCode != ctrl.codesSeen[0] {
		t.Errorf("lastTOTPCode = %q, want the code that was accepted (%q)", c.lastTOTPCode, ctrl.codesSeen[0])
	}
}

// The enrolment URL is what an operator has in hand after scanning the QR
// code, so it has to work as-is.
func TestLoginAcceptsOtpauthURLAsTheSecret(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed}
	srv := ctrl.server(t)

	if _, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", SkipTLSVerify: true,
		TOTPSecret: "otpauth://totp/Omada:admin?secret=" + rfc6238Seed + "&issuer=TP-Link",
	}); err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
}

func TestLoginWithoutTOTPSecretExplainsBothWaysOut(t *testing.T) {
	srv := (&mfaController{secret: rfc6238Seed}).server(t)

	_, err := NewClient(context.Background(), srv.URL, "admin", "secret", true)
	if err == nil {
		t.Fatal("login succeeded without a TOTP secret, want an error")
	}
	for _, want := range []string{"-30165", "totp_secret", "Account Security"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoginRejectsEmailOnlyAccount(t *testing.T) {
	srv := (&mfaController{secret: rfc6238Seed, supportedMFATypes: []int{mfaTypeEmail}}).server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("login succeeded for an email-only account, want an error")
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error %q does not name the account's actual 2FA method", err)
	}
}

// A code the controller has already seen — spent by the previous terraform
// command, or by a browser login — must not fail the run. The controller does
// not say how many attempts remain (verified live on 6.1.0.19), so the retry
// is bounded at one rather than gated on a budget that never arrives.
//
// A 1-second code step keeps this honest about waiting for a fresh window
// without making the test take 30 seconds.
func TestLoginRetriesOnceWithAFreshCode(t *testing.T) {
	fast := "otpauth://totp/x?secret=" + rfc6238Seed + "&period=1"
	ctrl := &mfaController{secret: fast, rejectFirstCode: true}
	srv := ctrl.server(t)

	c, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: fast, SkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
	if c.token != "tok-2fa" {
		t.Errorf("token = %q, want a session after retrying", c.token)
	}
	if len(ctrl.codesSeen) != 2 {
		t.Fatalf("sent %d codes, want 2 (the spent one, then a fresh one)", len(ctrl.codesSeen))
	}
	if ctrl.codesSeen[0] == ctrl.codesSeen[1] {
		t.Errorf("retried with the same code %q; the controller already refused it", ctrl.codesSeen[0])
	}
}

// Retrying is bounded: a genuinely wrong secret must not walk the account into
// a lock, one code per run.
func TestLoginStopsAfterTheRetry(t *testing.T) {
	fast := "otpauth://totp/x?secret=" + rfc6238Seed + "&period=1"
	ctrl := &mfaController{secret: fast, rejectCode: true}
	srv := ctrl.server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: fast, SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("login succeeded with a rejected code, want an error")
	}
	if len(ctrl.codesSeen) != 2 {
		t.Fatalf("sent %d codes, want exactly 2 — more walks the account into a lock", len(ctrl.codesSeen))
	}
	if !strings.Contains(err.Error(), "clock") {
		t.Errorf("error %q does not point at the likely causes", err)
	}
}

// When the controller *does* report its budget and it is nearly spent, stop at
// the first rejection instead of taking another attempt.
func TestLoginStopsRetryingWhenAttemptsRunLow(t *testing.T) {
	remaining := 2
	ctrl := &mfaController{secret: rfc6238Seed, rejectCode: true, remainAttempts: &remaining}
	srv := ctrl.server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("login succeeded with a rejected code, want an error")
	}
	if len(ctrl.codesSeen) != 1 {
		t.Errorf("sent %d codes; with the budget nearly gone it must stop at 1", len(ctrl.codesSeen))
	}
	if !strings.Contains(err.Error(), "1 attempts left") {
		t.Errorf("error %q does not report the controller's remaining attempts", err)
	}
}

// When the controller reports the account is already locked, stop immediately
// rather than spending the second attempt against a lock that is already in
// place.
func TestLoginStopsImmediatelyWhenAccountIsLocked(t *testing.T) {
	locked := 15
	ctrl := &mfaController{secret: rfc6238Seed, rejectCode: true, lockedMinutes: &locked}
	srv := ctrl.server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("login succeeded with a locked account, want an error")
	}
	if len(ctrl.codesSeen) != 1 {
		t.Errorf("sent %d codes; an already-locked account must not be retried", len(ctrl.codesSeen))
	}
	if !strings.Contains(err.Error(), "15 minute") {
		t.Errorf("error %q does not report the lockout duration", err)
	}
}

// A bad secret should fail while parsing, before the controller ever sees a
// code — a rejected code costs one of the account's few attempts.
func TestInvalidTOTPSecretFailsBeforeContactingTheController(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed}
	srv := ctrl.server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: "not base32 !", SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("client built with an invalid TOTP secret, want an error")
	}
	if ctrl.loginAttempts != 0 {
		t.Errorf("contacted the controller %d times, want 0", ctrl.loginAttempts)
	}
}

func TestNextTOTPCodeWaitsRatherThanReplaying(t *testing.T) {
	cfg, err := parseTOTPSecret(rfc6238Seed)
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}
	// A 1-second step keeps the test honest about waiting without making it slow.
	cfg.period = 1

	c := &Client{totp: cfg}
	first, err := c.nextTOTPCode(context.Background())
	if err != nil {
		t.Fatalf("nextTOTPCode: %v", err)
	}
	c.lastTOTPCode = first

	second, err := c.nextTOTPCode(context.Background())
	if err != nil {
		t.Fatalf("nextTOTPCode (second): %v", err)
	}
	if second == first {
		t.Errorf("replayed code %q instead of waiting for the next window", first)
	}
}

func TestNextTOTPCodeHonoursContextCancellation(t *testing.T) {
	cfg, err := parseTOTPSecret(rfc6238Seed)
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}
	c := &Client{totp: cfg}
	c.lastTOTPCode = cfg.codeAt(time.Now()) // force the wait

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.nextTOTPCode(ctx); err == nil {
		t.Fatal("nextTOTPCode ignored a cancelled context")
	}
}
