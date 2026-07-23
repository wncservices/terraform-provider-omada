// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// PSKSetting is the WPA/PSK sub-object. SecurityKey (the WiFi password) is
// write-only from Terraform's perspective: it is never surfaced into state, and
// updates deep-merge this object so an update that doesn't set a psk leaves the
// existing key untouched.
type PSKSetting struct {
	VersionPsk    int  `json:"versionPsk"`
	EncryptionPsk int  `json:"encryptionPsk"`
	GikRekey      bool `json:"gikRekeyPskEnable"`
}

// LimitToggles is the {down,up}LimitEnable pair used by the rate-limit objects.
type LimitToggles struct {
	DownLimitEnable bool `json:"downLimitEnable"`
	UpLimitEnable   bool `json:"upLimitEnable"`
}

// RateBeaconCtrl is the per-band rate/beacon control sub-object.
type RateBeaconCtrl struct {
	Rate2g       bool `json:"rate2gCtrlEnable"`
	Rate5g       bool `json:"rate5gCtrlEnable"`
	Rate6g       bool `json:"rate6gCtrlEnable"`
	ManageRate2g bool `json:"manageRateControl2gEnable"`
	ManageRate5g bool `json:"manageRateControl5gEnable"`
}

// MultiCastSetting is the multicast/broadcast control sub-object.
type MultiCastSetting struct {
	MultiCastEnable bool `json:"multiCastEnable"`
	ChannelUtil     int  `json:"channelUtil"`
	ArpCastEnable   bool `json:"arpCastEnable"`
	IPv6CastEnable  bool `json:"ipv6CastEnable"`
	FilterEnable    bool `json:"filterEnable"`
}

// WirelessNetwork is a wireless SSID within a WLAN group. Verified against a
// live v6.2 controller. Complex/derived sub-objects (vlanSetting, loadBalance,
// featureDescription) are not modelled and are preserved on update.
type WirelessNetwork struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WLANID     string `json:"wlanId"`
	Band       int    `json:"band"`     // bitmask: 1=2.4, 2=5, 4=6 (7=all)
	Security   int    `json:"security"` // 0=open, 3=WPA2/WPA3
	Broadcast  bool   `json:"broadcast"`
	VLANEnable bool   `json:"vlanEnable"`
	VLANID     int    `json:"vlanId"`
	GuestNet   bool   `json:"guestNetEnable"`

	PortalEnable       bool `json:"portalEnable"`
	AccessEnable       bool `json:"accessEnable"`
	WLANScheduleEnable bool `json:"wlanScheduleEnable"`
	MacFilterEnable    bool `json:"macFilterEnable"`
	Enable11r          bool `json:"enable11r"`
	PMFMode            int  `json:"pmfMode"`
	MLOEnable          bool `json:"mloEnable"`
	HidePwd            bool `json:"hidePwd"`
	WanAccess          bool `json:"wanAccess"`
	ProhibitWifiShare  bool `json:"prohibitWifiShare"`

	PSKSetting       PSKSetting       `json:"pskSetting"`
	RateLimit        LimitToggles     `json:"rateLimit"`
	SSIDRateLimit    LimitToggles     `json:"ssidRateLimit"`
	RateBeaconCtrl   RateBeaconCtrl   `json:"rateAndBeaconCtrl"`
	MultiCastSetting MultiCastSetting `json:"multiCastSetting"`
	DHCPOption82     struct {
		DhcpEnable bool `json:"dhcpEnable"`
	} `json:"dhcpOption82"`
}

func ssidsPath(siteID, groupID string) string {
	return fmt.Sprintf("/sites/%s/setting/wlans/%s/ssids", siteID, groupID)
}

// ListSSIDs returns all SSIDs in a WLAN group.
func (c *Client) ListSSIDs(ctx context.Context, siteID, groupID string) ([]WirelessNetwork, error) {
	return listAll[WirelessNetwork](ctx, c, "ssids", ssidsPath(siteID, groupID))
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

// ssidDeepKeys are sub-objects that must be merged, never replaced.
var ssidDeepKeys = []string{"pskSetting", "rateLimit", "ssidRateLimit", "rateAndBeaconCtrl", "multiCastSetting", "dhcpOption82"}

// CreateSSID creates an SSID (null result → resolved by name). psk, if given,
// is placed under pskSetting.securityKey.
func (c *Client) CreateSSID(ctx context.Context, siteID, groupID string, fields map[string]any, psk string) (*WirelessNetwork, error) {
	if psk != "" {
		ps, _ := fields["pskSetting"].(map[string]any)
		if ps == nil {
			ps = map[string]any{}
		}
		ps["securityKey"] = psk
		fields["pskSetting"] = ps
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

// UpdateSSID does a read-modify-write. Nested sub-objects are deep-merged, so
// unmodelled keys survive and — critically — the existing pskSetting.securityKey
// is retained unless a new psk is supplied.
func (c *Client) UpdateSSID(ctx context.Context, siteID, groupID, id string, fields map[string]any, psk string) (*WirelessNetwork, error) {
	cur, err := c.RawByID(ctx, ssidsPath(siteID, groupID), "id", id)
	if err != nil {
		return nil, err
	}
	mergeInto(cur, fields, ssidDeepKeys...)
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
