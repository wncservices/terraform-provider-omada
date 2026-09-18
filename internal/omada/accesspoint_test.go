// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"strings"
	"testing"
)

func newTestClientForAP(t *testing.T) *Client {
	t.Helper()
	srv := newTestController(t)
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), srv.URL, "admin", "secret", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// The two guards in UpdateAccessPoint exist because of behaviour observed on a
// live EAP670, and neither failure is visible from the controller's response:
// one is reported as success, the other destroys a neighbouring field. If these
// tests ever start failing because the guards were relaxed, re-read the type
// comment on AccessPoint before deciding the guard was the problem.

func TestUpdateAccessPointRefusesChannel(t *testing.T) {
	c := newTestClientForAP(t)

	for _, radio := range []string{"radioSetting2g", "radioSetting5g"} {
		err := c.UpdateAccessPoint(context.Background(), "site-1", "AA-BB-CC-DD-EE-FF", map[string]any{
			radio: map[string]any{
				"radioEnable":  true,
				"channelWidth": "6",
				"channel":      "36",
				"txPower":      28,
			},
		})
		if err == nil {
			t.Fatalf("%s.channel: expected a refusal, got nil", radio)
		}
		if !strings.Contains(err.Error(), "channel") {
			t.Fatalf("%s: error should name the offending key, got %q", radio, err)
		}
	}
}

func TestUpdateAccessPointAllowsRadioWithoutChannel(t *testing.T) {
	c := newTestClientForAP(t)

	// The same radio object minus channel is legitimate: tx power and channel
	// width both take effect.
	err := c.UpdateAccessPoint(context.Background(), "site-1", "AA-BB-CC-DD-EE-FF", map[string]any{
		"radioSetting5g": map[string]any{
			"radioEnable":  true,
			"channelWidth": "6",
			"txPower":      20,
			"txPowerLevel": 3,
		},
	})
	if err != nil {
		t.Fatalf("radio write without channel should be allowed, got %v", err)
	}
}

func TestUpdateAccessPointRefusesUnpairedMVLAN(t *testing.T) {
	c := newTestClientForAP(t)

	err := c.UpdateAccessPoint(context.Background(), "site-1", "AA-BB-CC-DD-EE-FF", map[string]any{
		"mvlanEnable": false,
	})
	if err == nil {
		t.Fatal("mvlanEnable alone: expected a refusal, got nil")
	}
	if !strings.Contains(err.Error(), "mvlanNetworkId") {
		t.Fatalf("error should name the field that gets destroyed, got %q", err)
	}

	// Paired, it is allowed: the caller has stated what the id should be.
	err = c.UpdateAccessPoint(context.Background(), "site-1", "AA-BB-CC-DD-EE-FF", map[string]any{
		"mvlanEnable":    false,
		"mvlanNetworkId": "net-1",
	})
	if err != nil {
		t.Fatalf("paired mvlan write should be allowed, got %v", err)
	}
}

func TestUpdateAccessPointEmptyIsNoop(t *testing.T) {
	c := newTestClientForAP(t)
	if err := c.UpdateAccessPoint(context.Background(), "site-1", "AA-BB-CC-DD-EE-FF", nil); err != nil {
		t.Fatalf("empty field set should be a no-op, got %v", err)
	}
}
