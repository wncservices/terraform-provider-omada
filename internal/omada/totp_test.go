// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors, SHA-1 column. The seed there is the ASCII
// string "12345678901234567890", which is this base32 secret.
const rfc6238Seed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPMatchesRFC6238Vectors(t *testing.T) {
	cfg, err := parseTOTPSecret(rfc6238Seed)
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}
	cfg.digits = 8 // the published vectors are 8-digit

	for _, tc := range []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	} {
		if got := cfg.codeAt(time.Unix(tc.unix, 0)); got != tc.want {
			t.Errorf("codeAt(%d) = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

func TestTOTPCodeIsStableWithinItsWindowAndChangesAfter(t *testing.T) {
	cfg, err := parseTOTPSecret(rfc6238Seed)
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}

	start := time.Unix(1700000000, 0) // 10s short of a boundary (1700000000 % 30 == 20)
	first := cfg.codeAt(start)
	if got := cfg.codeAt(start.Add(9 * time.Second)); got != first {
		t.Errorf("code changed inside its window: %s then %s", first, got)
	}

	next := cfg.nextWindow(start)
	if !next.After(start) || next.Sub(start) > 30*time.Second {
		t.Fatalf("nextWindow(%v) = %v, want within the next 30s", start, next)
	}
	if got := cfg.codeAt(next); got == first {
		t.Errorf("code did not change at the window boundary %v", next)
	}
}

func TestParseTOTPSecretAcceptsTheFormsPeopleActuallyPasteIn(t *testing.T) {
	want := mustCode(t, rfc6238Seed, 1700000000)

	for name, secret := range map[string]string{
		"lower case":     "gezdgnbvgy3tqojqgezdgnbvgy3tqojq",
		"spaced groups":  "GEZD GNBV GY3T QOJQ GEZD GNBV GY3T QOJQ",
		"hyphenated":     "GEZD-GNBV-GY3T-QOJQ-GEZD-GNBV-GY3T-QOJQ",
		"surrounded":     "  " + rfc6238Seed + "\n",
		"otpauth url":    "otpauth://totp/Omada:terraform?secret=" + rfc6238Seed + "&issuer=Omada",
		"otpauth params": "otpauth://totp/Omada:terraform?secret=" + rfc6238Seed + "&algorithm=SHA1&digits=6&period=30",
	} {
		t.Run(name, func(t *testing.T) {
			if got := mustCode(t, secret, 1700000000); got != want {
				t.Errorf("code = %s, want %s", got, want)
			}
		})
	}
}

// digits=10 is the otpauth maximum parseTOTPSecret allows, and 10^10 overflows
// a uint32 modulus (wraps to 1,410,065,408), so this only passes with a wider
// modulus. Expected value computed independently with math/big, not by reading
// codeAt's own arithmetic back.
func TestTOTPCodeAtTenDigitsDoesNotOverflow(t *testing.T) {
	cfg, err := parseTOTPSecret(rfc6238Seed)
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}
	cfg.digits = 10

	if got, want := cfg.codeAt(time.Unix(1111111109, 0)), "0907081804"; got != want {
		t.Errorf("codeAt with digits=10 = %s, want %s", got, want)
	}
}

func TestParseTOTPSecretHonoursOtpauthParameters(t *testing.T) {
	cfg, err := parseTOTPSecret("otpauth://totp/x?secret=" + rfc6238Seed + "&digits=8&period=60&algorithm=SHA256")
	if err != nil {
		t.Fatalf("parseTOTPSecret: %v", err)
	}
	if cfg.digits != 8 || cfg.period != 60 {
		t.Errorf("digits/period = %d/%d, want 8/60", cfg.digits, cfg.period)
	}
	// Computed independently (HMAC-SHA256 over T=1111111109/60, 8 digits), so
	// that mis-wiring the algorithm or the step is caught rather than agreeing
	// with itself.
	if got := cfg.codeAt(time.Unix(1111111109, 0)); got != "69648066" {
		t.Errorf("SHA-256/60s code = %s, want 69648066", got)
	}
}

func TestParseTOTPSecretRejectsGarbage(t *testing.T) {
	// nolint below: gosec's G101 sees `secret=` in the otpauth fixtures. These
	// are deliberately malformed inputs for the parser's error paths.
	for name, input := range map[string]string{ //nolint:gosec // test fixtures, not real credentials
		"empty":               "",
		"blank":               "   ",
		"not base32":          "not-a-secret-1!",
		"otpauth no secret":   "otpauth://totp/Omada:terraform?issuer=Omada",
		"otpauth bad digits":  "otpauth://totp/x?secret=" + rfc6238Seed + "&digits=99",
		"otpauth bad period":  "otpauth://totp/x?secret=" + rfc6238Seed + "&period=0",
		"otpauth bad algo":    "otpauth://totp/x?secret=" + rfc6238Seed + "&algorithm=MD5",
		"otpauth bad base32":  "otpauth://totp/x?secret=!!!!",
		"decodes to nothing":  "=",
		"invalid padding run": "A",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTOTPSecret(input); err == nil {
				t.Errorf("parseTOTPSecret(%q) succeeded, want an error", input)
			}
		})
	}
}

func mustCode(t *testing.T, secret string, unix int64) string {
	t.Helper()
	cfg, err := parseTOTPSecret(secret)
	if err != nil {
		t.Fatalf("parseTOTPSecret(%q): %v", secret, err)
	}
	return cfg.codeAt(time.Unix(unix, 0))
}
