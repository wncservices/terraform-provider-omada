// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Gateway device settings (Devices -> the gateway -> Config).
//
// Verified against a live v6.2 controller:
//
//	GET   /sites/{site}/gateways/{mac}   full device document
//	PATCH /sites/{site}/gateways/{mac}   partial update (PUT/POST answer -1600)
//
// PATCH genuinely is partial: a body carrying one key changes that key and
// leaves the rest alone. Every field modelled below was confirmed accepted by
// writing the gateway its own current value and checking nothing moved.
//
// Unlike the switch, the gateway does *not* need the Open API. Its Open API
// document (`/openapi/v1/.../gateways/{mac}`) exists but carries only telemetry
// — cpu, memory, temperature, uptime, port stats — and none of the settings
// here, so there is nothing to cross surfaces for.
//
// **`portConfigs` is deliberately not modelled.** Those are the gateway's own
// physical ports, which is where the WAN lives: a wrong write there does not
// misconfigure a feature, it takes the site off the internet. WAN settings are
// reachable through `omada_wan` and the disable-NAT/port-forward resources,
// which are scoped and reviewable; a general-purpose per-port writer on the
// gateway is not worth the failure mode.
type Gateway struct {
	MAC   string `json:"mac"`
	Name  string `json:"name"`
	Model string `json:"model"`
	IP    string `json:"ip"`

	// LEDSetting is the controller enum for the device LED: observed 2 on a
	// gateway following the site setting.
	LEDSetting int `json:"ledSetting"`

	LLDPEnable  bool `json:"lldpEnable"`
	LLDPSetting int  `json:"lldpSetting"`

	// HWOffloadEnable turns on hardware acceleration of forwarding. It is
	// generally wanted, but it bypasses parts of the software path, so some
	// inspection features quietly stop applying when it is on.
	HWOffloadEnable bool `json:"hwOffloadEnable"`

	// IPPT is IP-passthrough. SupportIPPT reports whether the hardware offers
	// it at all — the ER707-M2 does not.
	IPPT        bool `json:"ippt"`
	SupportIPPT bool `json:"supportIppt"`

	SNMPSetting *struct {
		Location string `json:"location"`
		Contact  string `json:"contact"`
	} `json:"snmpSeting,omitempty"` // sic: the controller misspells it

	IPTVSetting *struct {
		IGMPEnable  bool   `json:"igmpEnable"`
		IGMPVersion string `json:"igmpVersion"`
	} `json:"iptvSetting,omitempty"`
}

func gatewayPath(siteID, mac string) string {
	return fmt.Sprintf("/sites/%s/gateways/%s", siteID, NormalizeMAC(mac))
}

// GetGateway returns the gateway's device document.
func (c *Client) GetGateway(ctx context.Context, siteID, mac string) (*Gateway, error) {
	var gw Gateway
	if err := c.Do(ctx, http.MethodGet, gatewayPath(siteID, mac), nil, &gw); err != nil {
		return nil, fmt.Errorf("reading gateway %s: %w", mac, err)
	}
	return &gw, nil
}

// UpdateGateway applies a partial set of device settings.
//
// The caller passes only what it means to change. Nothing is read first and
// merged, because PATCH here is already partial — and on a gateway, sending
// back a document that was read a moment ago is exactly how an unrelated
// setting gets reverted by a concurrent change in the UI.
func (c *Client) UpdateGateway(ctx context.Context, siteID, mac string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	for k := range fields {
		if k == "portConfigs" || k == "portGeneralConfigs" {
			// Belt and braces: nothing in this provider builds such a body, and
			// nothing should start doing so by accident.
			return fmt.Errorf("refusing to write %q on the gateway: its physical port "+
				"configuration is out of scope for this resource", k)
		}
	}
	if err := c.Do(ctx, http.MethodPatch, gatewayPath(siteID, mac), fields, nil); err != nil {
		return fmt.Errorf("updating gateway %s: %w", mac, err)
	}
	return nil
}
