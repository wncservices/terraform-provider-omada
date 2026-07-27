// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// DNS proxy / DNS-over-HTTPS settings (Settings -> Network Security -> DNS).
//
// Verified against a live v6.2 controller:
//
//	GET /sites/{site}/setting/dns-proxy
//	PUT /sites/{site}/setting/dns-proxy   (PATCH and POST answer -1600)
//
// The document has two server lists with different ownership, and the
// distinction is the whole design of this resource:
//
//   - `defaultServers` is a **fixed list built into the firmware** — five
//     well-known DoH providers identified by an opaque `type`. The list cannot
//     be added to or removed from; only each entry's `enable` flips.
//   - `customizedServers` is **yours** — name plus one or more DoH URLs, full
//     create/update/delete within the document.
//
// So the resource manages the set of enabled default types, and the customised
// list in full, and never rewrites the default list itself.
const dnsProxyPath = "/setting/dns-proxy"

// DoHCustomServer is one user-defined DNS-over-HTTPS entry.
type DoHCustomServer struct {
	Name    string   `json:"name"`
	Enable  bool     `json:"enable"`
	Servers []string `json:"servers"`
}

// DoHDefaultServer is one firmware-provided entry.
type DoHDefaultServer struct {
	Type   int  `json:"type"`
	Enable bool `json:"enable"`
}

// DNSProxy is the modelled document.
type DNSProxy struct {
	Enable bool

	// EnabledDefaultTypes are the `type` values whose enable is true.
	EnabledDefaultTypes []int
	// DefaultServers is the full firmware list as read, for reporting.
	DefaultServers []DoHDefaultServer

	CustomServers []DoHCustomServer

	// Controller-owned.
	SupportDNSOverride bool
	DoHServerLimit     int
	DoTServerLimit     int
}

type dnsProxyDoc struct {
	Enable     bool `json:"enable"`
	DoHSetting struct {
		DefaultServers    []DoHDefaultServer `json:"defaultServers"`
		CustomizedServers []DoHCustomServer  `json:"customizedServers"`
	} `json:"dohSetting"`
	SupportDNSOverride bool `json:"supportDnsOverride"`
	DoHServerLimit     int  `json:"dohServerLimit"`
	DoTServerLimit     int  `json:"dotServerLimit"`
}

func dnsProxySitePath(siteID string) string {
	return fmt.Sprintf("/sites/%s%s", siteID, dnsProxyPath)
}

// GetDNSProxy reads the DNS proxy document.
func (c *Client) GetDNSProxy(ctx context.Context, siteID string) (*DNSProxy, error) {
	var doc dnsProxyDoc
	if err := c.Do(ctx, http.MethodGet, dnsProxySitePath(siteID), nil, &doc); err != nil {
		return nil, fmt.Errorf("reading dns proxy settings: %w", err)
	}
	out := &DNSProxy{
		Enable:             doc.Enable,
		DefaultServers:     doc.DoHSetting.DefaultServers,
		CustomServers:      doc.DoHSetting.CustomizedServers,
		SupportDNSOverride: doc.SupportDNSOverride,
		DoHServerLimit:     doc.DoHServerLimit,
		DoTServerLimit:     doc.DoTServerLimit,
	}
	for _, d := range doc.DoHSetting.DefaultServers {
		if d.Enable {
			out.EnabledDefaultTypes = append(out.EnabledDefaultTypes, d.Type)
		}
	}
	return out, nil
}

// UpdateDNSProxy writes the modelled fields back.
//
// The default-server list is read first and only each entry's flag is changed,
// because the list is the firmware's. A type that is not in it is an error
// rather than a silent no-op: the controller accepts such a write and drops it,
// leaving a configuration that claims a resolver is on when it never will be.
func (c *Client) UpdateDNSProxy(ctx context.Context, siteID string, in DNSProxy) error {
	cur, err := c.GetDNSProxy(ctx, siteID)
	if err != nil {
		return err
	}

	want := map[int]bool{}
	for _, t := range in.EnabledDefaultTypes {
		want[t] = true
	}
	known := map[int]bool{}
	for _, d := range cur.DefaultServers {
		known[d.Type] = true
	}
	for t := range want {
		if !known[t] {
			return fmt.Errorf("default DoH server type %d is not one this firmware offers", t)
		}
	}

	defaults := make([]DoHDefaultServer, len(cur.DefaultServers))
	copy(defaults, cur.DefaultServers)
	for i := range defaults {
		defaults[i].Enable = want[defaults[i].Type]
	}

	if cur.DoHServerLimit > 0 && len(in.CustomServers) > cur.DoHServerLimit {
		return fmt.Errorf("%d customised DoH servers exceeds this controller's limit of %d",
			len(in.CustomServers), cur.DoHServerLimit)
	}
	custom := in.CustomServers
	if custom == nil {
		custom = []DoHCustomServer{}
	}
	for i := range custom {
		if len(custom[i].Servers) == 0 {
			return fmt.Errorf("customised DoH server %q has no urls", custom[i].Name)
		}
		custom[i].Servers = nilToEmptyStrings(custom[i].Servers)
	}

	body := map[string]any{
		"enable": in.Enable,
		"dohSetting": map[string]any{
			"defaultServers":    defaults,
			"customizedServers": custom,
		},
	}
	if err := c.Do(ctx, http.MethodPut, dnsProxySitePath(siteID), body, nil); err != nil {
		return fmt.Errorf("updating dns proxy settings: %w", err)
	}
	return nil
}
