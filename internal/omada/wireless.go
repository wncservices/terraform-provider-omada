// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// WirelessNetwork is a wireless SSID within a WLAN group. Verified against a
// live v6.2 controller (/setting/wlans/{groupId}/ssids). This provider manages
// a subset of fields; the rest are preserved on update (read-modify-write).
type WirelessNetwork struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WLANID     string `json:"wlanId"`
	Band       int    `json:"band"`     // 1=2.4, 2=5, 4=6, bitmask (7=all)
	Security   int    `json:"security"` // 0=open, 3=WPA2/WPA3
	Broadcast  bool   `json:"broadcast"`
	VLANEnable bool   `json:"vlanEnable"`
	VLANID     int    `json:"vlanId"`
	GuestNet   bool   `json:"guestNetEnable"`
	PSKSetting struct {
		SecurityKey string `json:"securityKey"`
	} `json:"pskSetting"`
}

func ssidsPath(siteID, groupID string) string {
	return fmt.Sprintf("/sites/%s/setting/wlans/%s/ssids", siteID, groupID)
}

func ssidsListPath(siteID, groupID string) string {
	return ssidsPath(siteID, groupID) + "?currentPage=1&currentPageSize=1000"
}

// ListSSIDs returns all SSIDs in a WLAN group.
func (c *Client) ListSSIDs(ctx context.Context, siteID, groupID string) ([]WirelessNetwork, error) {
	var out struct {
		Data []WirelessNetwork `json:"data"`
	}
	if err := c.Do(ctx, "GET", ssidsListPath(siteID, groupID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing ssids: %w", err)
	}
	return out.Data, nil
}

// GetSSID returns one SSID by id.
func (c *Client) GetSSID(ctx context.Context, siteID, groupID, id string) (*WirelessNetwork, error) {
	ssids, err := c.ListSSIDs(ctx, siteID, groupID)
	if err != nil {
		return nil, err
	}
	for i := range ssids {
		if ssids[i].ID == id {
			return &ssids[i], nil
		}
	}
	return nil, fmt.Errorf("ssid %q not found in group %q", id, groupID)
}

// CreateSSID creates an SSID (null result → resolved by name). psk (if given)
// is placed under pskSetting.securityKey.
func (c *Client) CreateSSID(ctx context.Context, siteID, groupID string, fields map[string]any, psk string) (*WirelessNetwork, error) {
	if psk != "" {
		fields["pskSetting"] = map[string]any{"securityKey": psk, "versionPsk": 3}
	}
	name, _ := fields["name"].(string)
	if err := c.Do(ctx, "POST", ssidsPath(siteID, groupID), fields, nil); err != nil {
		return nil, fmt.Errorf("creating ssid: %w", err)
	}
	ssids, err := c.ListSSIDs(ctx, siteID, groupID)
	if err != nil {
		return nil, err
	}
	for i := range ssids {
		if ssids[i].Name == name {
			return &ssids[i], nil
		}
	}
	return nil, fmt.Errorf("ssid %q not found after create", name)
}

// UpdateSSID does a read-modify-write, merging managed fields (and the psk into
// the existing pskSetting so other PSK fields are preserved) and PATCHing back.
func (c *Client) UpdateSSID(ctx context.Context, siteID, groupID, id string, fields map[string]any, psk string) (*WirelessNetwork, error) {
	cur, err := c.RawByID(ctx, ssidsListPath(siteID, groupID), "id", id)
	if err != nil {
		return nil, err
	}
	for k, v := range fields {
		cur[k] = v
	}
	if psk != "" {
		ps, _ := cur["pskSetting"].(map[string]any)
		if ps == nil {
			ps = map[string]any{}
		}
		ps["securityKey"] = psk
		cur["pskSetting"] = ps
	}
	if err := c.Do(ctx, "PATCH", ssidsPath(siteID, groupID)+"/"+id, cur, nil); err != nil {
		return nil, fmt.Errorf("updating ssid %q: %w", id, err)
	}
	return c.GetSSID(ctx, siteID, groupID, id)
}

// DeleteSSID removes an SSID.
func (c *Client) DeleteSSID(ctx context.Context, siteID, groupID, id string) error {
	if err := c.Do(ctx, "DELETE", ssidsPath(siteID, groupID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting ssid %q: %w", id, err)
	}
	return nil
}
