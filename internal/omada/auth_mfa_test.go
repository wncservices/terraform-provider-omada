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

	// supportedMFATypes is what the challenge advertises. Empty means the
	// field is omitted, as some builds do.
	supportedMFATypes []int

	// rejectCode forces the "wrong code" answer, for the failure paths.
	rejectCode bool

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
		result := map[string]any{"MFAId": "mfa-1"}
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

		if body.MFAId != "mfa-1" || body.MFAType != mfaTypeTOTP || body.Username != "admin" || body.Password != "secret" {
			writeEnvelope(w, -30109, "Invalid username or password.", nil)
			return
		}

		cfg, err := parseTOTPSecret(m.secret)
		if err != nil {
			t.Errorf("fixture secret: %v", err)
		}
		if m.rejectCode || body.Code != cfg.codeAt(time.Now()) {
			writeEnvelope(w, -30139, "Invalid code.", map[string]any{"codeRemainAttempts": 4})
			return
		}
		writeEnvelope(w, 0, "", map[string]any{"token": "tok-2fa", "roleType": 0})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginAnswersTOTPChallenge(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed, supportedMFATypes: []int{mfaTypeTOTP}}
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

func TestLoginDoesNotRetryARejectedCode(t *testing.T) {
	ctrl := &mfaController{secret: rfc6238Seed, rejectCode: true}
	srv := ctrl.server(t)

	_, err := NewClientWithConfig(context.Background(), Config{
		URL: srv.URL, Username: "admin", Password: "secret", TOTPSecret: rfc6238Seed, SkipTLSVerify: true,
	})
	if err == nil {
		t.Fatal("login succeeded with a rejected code, want an error")
	}
	if len(ctrl.codesSeen) != 1 {
		t.Fatalf("sent %d codes; a rejected code must not be retried, the controller locks the account", len(ctrl.codesSeen))
	}
	if !strings.Contains(err.Error(), "clock") {
		t.Errorf("error %q does not point at the likely causes", err)
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
