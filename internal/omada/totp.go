// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 specifies HMAC-SHA1 for TOTP; the controller's enrolment URL uses it
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TOTP (RFC 6238) is what the controller's 2FA enrolment hands out: the QR code
// on the login page encodes an `otpauth://totp/...` URL, and an authenticator
// app turns its `secret` into the six-digit code the login form asks for. This
// file is that same computation, so the provider can log in to a controller
// with 2FA enforced. See auth.go for the login exchange it feeds.

// totpConfig is a parsed enrolment secret plus the parameters that decide how
// its codes are derived. The defaults are the ones RFC 6238 specifies and every
// authenticator app assumes: SHA-1, 6 digits, a new code every 30 seconds.
type totpConfig struct {
	secret  []byte
	digits  int
	period  int
	newHash func() hash.Hash
}

// parseTOTPSecret accepts either a bare base32 secret ("JBSWY3DPEHPK3PXP", as
// an authenticator app displays it) or the whole `otpauth://totp/...` URL the
// controller's QR code encodes. Spaces, hyphens, lower case and missing padding
// are all tolerated, because that is how the secret gets copied by hand.
func parseTOTPSecret(raw string) (*totpConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty TOTP secret")
	}

	cfg := &totpConfig{digits: 6, period: 30, newHash: sha1.New}
	secret := raw

	if strings.HasPrefix(strings.ToLower(raw), "otpauth://") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parsing otpauth url: %w", err)
		}
		q := u.Query()
		secret = q.Get("secret")
		if secret == "" {
			return nil, fmt.Errorf("otpauth url has no secret parameter")
		}
		if v := q.Get("digits"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 6 || n > 10 {
				return nil, fmt.Errorf("unsupported otpauth digits %q", v)
			}
			cfg.digits = n
		}
		if v := q.Get("period"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("unsupported otpauth period %q", v)
			}
			cfg.period = n
		}
		switch strings.ToUpper(q.Get("algorithm")) {
		case "", "SHA1":
		case "SHA256":
			cfg.newHash = sha256.New
		case "SHA512":
			cfg.newHash = sha512.New
		default:
			return nil, fmt.Errorf("unsupported otpauth algorithm %q", q.Get("algorithm"))
		}
	}

	key, err := decodeBase32Secret(secret)
	if err != nil {
		return nil, err
	}
	cfg.secret = key
	return cfg, nil
}

// decodeBase32Secret normalises the many ways a base32 secret gets written down
// before decoding it.
func decodeBase32Secret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(s))
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("TOTP secret is not valid base32: %w", err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("TOTP secret decoded to zero bytes")
	}
	return key, nil
}

// codeAt derives the code for the counter window containing t, by RFC 4226
// dynamic truncation of an HMAC over the window number.
func (t *totpConfig) codeAt(at time.Time) string {
	counter := uint64(at.Unix()) / uint64(t.period) //nolint:gosec // pre-1970 clocks are not a case worth handling

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(t.newHash, t.secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := uint64(binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff)

	// uint64, not uint32: digits can be up to 10 (see parseTOTPSecret), and
	// 10^10 overflows a uint32.
	mod := uint64(1)
	for range t.digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", t.digits, value%mod)
}

// nextWindow returns when the code covering t expires, which is when a fresh
// code becomes available. Used to avoid presenting the same code twice.
func (t *totpConfig) nextWindow(at time.Time) time.Time {
	period := int64(t.period)
	return time.Unix((at.Unix()/period+1)*period, 0)
}
