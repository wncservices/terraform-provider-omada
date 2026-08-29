// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
)

// WANIPv6Dynamic is the prefix-delegation sub-config used when
// WANIPv6Setting.Proto is "dynamic". Verified live on a WAN receiving a /48
// delegation (pdSize 48); other proto values ("static", "pppoe", ...) are
// unverified and use payload shapes this type does not model.
type WANIPv6Dynamic struct {
	GetIPv6     string `json:"getIpv6"`     // e.g. "auto"
	GetIPv6Type int    `json:"getIpv6Type"` // controller enum; 3 observed live, undocumented by TP-Link
	Prefix      int    `json:"prefix"`      // request prefix delegation from the ISP; 1 observed live (on)
	PDSize      int    `json:"pdSize"`      // requested delegated prefix size in bits, e.g. 48
	DNS         string `json:"dns"`         // e.g. "dynamic"
	DNSType     int    `json:"dnsType"`
}

// WANIPv6Setting is one WAN port's IPv6 connection config (wanPortIpv6Setting,
// nested inside GET /setting/wan/networks -> wanPortSettings[]).
//
// The read shape is confirmed against a live controller. The WRITE path is
// NOT: this codebase had never attempted a WAN write before this type existed
// (see DESIGN.md §5.3, "Writable WAN" — a deliberate non-goal, because the
// only WAN is the live one and a bad write drops the site's internet
// connection, not just a VLAN's). The verb and endpoint in
// UpdateWANIPv6Setting (PATCH to the WAN port's item path, mirroring
// UpdateNetwork) are inferred from this codebase's convention for nested
// settings documents — not live-validated. Confirm on real hardware,
// narrowly and deliberately (a no-op write-back first), before trusting this
// in an unattended apply.
type WANIPv6Setting struct {
	PortUUID  string          `json:"portUuid"`
	Enable    int             `json:"enable"`
	Proto     string          `json:"proto"` // e.g. "dynamic"; only this value is verified
	ProtoType int             `json:"protoType"`
	Dynamic   *WANIPv6Dynamic `json:"ipv6Dynamic,omitempty"`
}

func wanNetworksPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/wan/networks", siteID)
}

// rawWANPort fetches one WAN port's full raw document (every sub-setting),
// keyed by portUuid. GET /setting/wan/networks answers a single object
// ({"wanPortSettings": [...]}), not the paginated {data: [...]} shape RawList
// expects, so this reads it directly rather than through that helper.
func (c *Client) rawWANPort(ctx context.Context, siteID, portUUID string) (map[string]any, error) {
	var raw map[string]any
	if err := c.Do(ctx, "GET", wanNetworksPath(siteID), nil, &raw); err != nil {
		return nil, fmt.Errorf("reading wan settings: %w", err)
	}
	list, _ := raw["wanPortSettings"].([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if uuid, _ := m["portUuid"].(string); uuid == portUUID {
			return m, nil
		}
	}
	return nil, fmt.Errorf("wan port %q not found", portUUID)
}

// GetWANIPv6Setting returns one WAN port's IPv6 connection config.
func (c *Client) GetWANIPv6Setting(ctx context.Context, siteID, portUUID string) (*WANIPv6Setting, error) {
	raw, err := c.rawWANPort(ctx, siteID, portUUID)
	if err != nil {
		return nil, err
	}
	v6raw, ok := raw["wanPortIpv6Setting"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("wan port %q has no wanPortIpv6Setting", portUUID)
	}
	data, err := json.Marshal(v6raw)
	if err != nil {
		return nil, fmt.Errorf("re-marshalling wan ipv6 setting: %w", err)
	}
	var v6 WANIPv6Setting
	if err := json.Unmarshal(data, &v6); err != nil {
		return nil, fmt.Errorf("decoding wan ipv6 setting: %w", err)
	}
	return &v6, nil
}

// UpdateWANIPv6Setting replaces one WAN port's IPv6 sub-document, through a
// read-modify-write of the full port document so wanPortIpv4Setting and
// wanPortMacSetting are preserved untouched — this deliberately never writes
// anything outside wanPortIpv6Setting. See the WANIPv6Setting doc comment:
// the write verb/endpoint here are inferred, not live-validated.
func (c *Client) UpdateWANIPv6Setting(ctx context.Context, siteID, portUUID string, in WANIPv6Setting) (*WANIPv6Setting, error) {
	cur, err := c.rawWANPort(ctx, siteID, portUUID)
	if err != nil {
		return nil, err
	}
	in.PortUUID = portUUID
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshalling wan ipv6 setting: %w", err)
	}
	var v6 map[string]any
	if err := json.Unmarshal(data, &v6); err != nil {
		return nil, fmt.Errorf("re-decoding wan ipv6 setting: %w", err)
	}
	cur["wanPortIpv6Setting"] = v6

	path := fmt.Sprintf("%s/%s", wanNetworksPath(siteID), portUUID)
	if err := c.Do(ctx, "PATCH", path, cur, nil); err != nil {
		return nil, fmt.Errorf("updating wan ipv6 setting: %w", err)
	}
	return c.GetWANIPv6Setting(ctx, siteID, portUUID)
}
