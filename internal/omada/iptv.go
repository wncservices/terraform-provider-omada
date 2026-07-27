// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// IPTV settings (Settings -> Network Security -> IPTV, and IGMP proxy).
//
// Verified against a live v6.2 controller:
//
//	GET /sites/{site}/setting/iptv
//	PUT /sites/{site}/setting/iptv   (PATCH and POST answer -1600)
//
// The document is two nested objects. `iptvSetting.portConfig` is a fixed list
// of the gateway's WAN/LAN ports, each with a `status` flag and controller-owned
// capability fields (`supportIptv`, `existIptv`) — the list itself is not
// editable, only the flags on it, so IPTVSettings carries just the set of port
// ids that are switched on and the read-modify-write preserves the rest.
const iptvPath = "/setting/iptv"

// IPTVPort is one row of the fixed port list.
type IPTVPort struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Port        string `json:"port"`
	Status      bool   `json:"status"`
	SupportIPTV bool   `json:"supportIptv"`
	ExistIPTV   bool   `json:"existIptv"`
}

// IPTVSettings is the modelled subset of the document.
type IPTVSettings struct {
	IGMPProxyEnable bool
	IGMPVersion     string
	IPTVEnable      bool
	// EnabledPortIDs are the `port` ids whose status is true.
	EnabledPortIDs []string
	// Ports is the full list as read, for reporting.
	Ports []IPTVPort
}

type iptvDoc struct {
	IGMPSetting struct {
		IGMPProxyEnable bool   `json:"igmpProxyEnable"`
		IGMPVersion     string `json:"igmpVersion"`
	} `json:"igmpSetting"`
	IPTVSetting struct {
		IPTVEnable bool       `json:"iptvEnable"`
		PortConfig []IPTVPort `json:"portConfig"`
	} `json:"iptvSetting"`
}

func iptvSitePath(siteID string) string {
	return fmt.Sprintf("/sites/%s%s", siteID, iptvPath)
}

// GetIPTV reads the IPTV document.
func (c *Client) GetIPTV(ctx context.Context, siteID string) (*IPTVSettings, error) {
	var doc iptvDoc
	if err := c.Do(ctx, http.MethodGet, iptvSitePath(siteID), nil, &doc); err != nil {
		return nil, fmt.Errorf("reading iptv settings: %w", err)
	}
	out := &IPTVSettings{
		IGMPProxyEnable: doc.IGMPSetting.IGMPProxyEnable,
		IGMPVersion:     doc.IGMPSetting.IGMPVersion,
		IPTVEnable:      doc.IPTVSetting.IPTVEnable,
		Ports:           doc.IPTVSetting.PortConfig,
	}
	for _, p := range doc.IPTVSetting.PortConfig {
		if p.Status {
			out.EnabledPortIDs = append(out.EnabledPortIDs, p.Port)
		}
	}
	return out, nil
}

// UpdateIPTV writes the modelled fields back.
//
// The port list is read first and only each row's `status` is changed, because
// the rows themselves are the gateway's physical ports: their ids, names and
// capability flags belong to the controller, and inventing a list would at best
// be rejected and at worst apply IPTV mode to the wrong port.
//
// A port id that is not in the controller's list is an error rather than a
// silent no-op — it almost always means a stale id copied from another site.
func (c *Client) UpdateIPTV(ctx context.Context, siteID string, in IPTVSettings) error {
	cur, err := c.GetIPTV(ctx, siteID)
	if err != nil {
		return err
	}

	want := map[string]bool{}
	for _, id := range in.EnabledPortIDs {
		want[id] = true
	}
	known := map[string]bool{}
	for _, p := range cur.Ports {
		known[p.Port] = true
	}
	for id := range want {
		if !known[id] {
			return fmt.Errorf("iptv port %q is not one of this gateway's ports", id)
		}
	}

	ports := make([]IPTVPort, len(cur.Ports))
	copy(ports, cur.Ports)
	for i := range ports {
		ports[i].Status = want[ports[i].Port]
	}

	body := map[string]any{
		"igmpSetting": map[string]any{
			"igmpProxyEnable": in.IGMPProxyEnable,
			"igmpVersion":     in.IGMPVersion,
		},
		"iptvSetting": map[string]any{
			"iptvEnable": in.IPTVEnable,
			"portConfig": ports,
		},
	}
	if err := c.Do(ctx, http.MethodPut, iptvSitePath(siteID), body, nil); err != nil {
		return fmt.Errorf("updating iptv settings: %w", err)
	}
	return nil
}
