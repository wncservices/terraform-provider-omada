// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// WANPort is the configuration of one gateway WAN port, flattened from
// /setting/wan/networks -> wanPortSettings[].
//
// This is exposed read-only (a data source). The WAN endpoint's write path is
// deliberately not implemented: a bad WAN write takes the internet down, and it
// cannot be validated safely on a live household gateway.
type WANPort struct {
	PortUUID     string
	PortName     string
	Proto        string // "dhcp" | "pppoe" | "static" | ...
	VLANID       int
	QosTagEnable bool
	MTU          int
	MacMethod    string
	MacAddress   string
	IPv6Enable   int
}

// GetWANPorts returns the gateway's WAN port configuration.
func (c *Client) GetWANPorts(ctx context.Context, siteID string) ([]WANPort, error) {
	var raw map[string]any
	if err := c.Do(ctx, "GET", fmt.Sprintf("/sites/%s/setting/wan/networks", siteID), nil, &raw); err != nil {
		return nil, fmt.Errorf("reading wan settings: %w", err)
	}
	list, _ := raw["wanPortSettings"].([]any)
	out := make([]WANPort, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		p := WANPort{}
		p.PortUUID, _ = m["portUuid"].(string)
		p.PortName, _ = m["portName"].(string)
		if v4, ok := m["wanPortIpv4Setting"].(map[string]any); ok {
			p.Proto, _ = v4["proto"].(string)
			if f, ok := v4["vlanId"].(float64); ok {
				p.VLANID = int(f)
			}
			p.QosTagEnable, _ = v4["qosTagEnable"].(bool)
			if dhcp, ok := v4["ipv4Dhcp"].(map[string]any); ok {
				if f, ok := dhcp["mtu"].(float64); ok {
					p.MTU = int(f)
				}
			}
		}
		if mac, ok := m["wanPortMacSetting"].(map[string]any); ok {
			p.MacMethod, _ = mac["method"].(string)
			p.MacAddress, _ = mac["mac"].(string)
		}
		if v6, ok := m["wanPortIpv6Setting"].(map[string]any); ok {
			if f, ok := v6["enable"].(float64); ok {
				p.IPv6Enable = int(f)
			}
		}
		out = append(out, p)
	}
	return out, nil
}
