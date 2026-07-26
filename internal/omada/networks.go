// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// EnableFlag is the common {"enable": bool} sub-object.
type EnableFlag struct {
	Enable bool `json:"enable"`
}

// IntEnable is the {"enable": int} variant used by lanNetworkIpv6Config.
type IntEnable struct {
	Enable int `json:"enable"`
}

// DHCPOption is a DHCP option handed out on a network (e.g. code 138 -> a
// controller/AP address).
type DHCPOption struct {
	Code  int    `json:"code"`
	Type  int    `json:"type"`
	Value string `json:"value"`
}

// DHCPSettings is the nested DHCP server config. Note: distinct from dhcpGuard,
// which is rogue-DHCP protection.
type DHCPSettings struct {
	Enable      bool         `json:"enable"`
	IPAddrStart string       `json:"ipaddrStart"`
	IPAddrEnd   string       `json:"ipaddrEnd"`
	LeaseTime   int          `json:"leasetime"`
	DNSMode     string       `json:"dhcpns"`
	Options     []DHCPOption `json:"options"`
}

// Network is a LAN network (VLAN interface) on a site. Field mappings verified
// against a live v6.2 controller (GET /sites/{id}/setting/lan/networks).
// Derived/read-only fields (ipRangeStart/End, ipRangePool, totalIpNum, state,
// deviceMac, ...) are intentionally not modelled; UpdateNetwork preserves them.
type Network struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Purpose       string   `json:"purpose"` // "interface" | "vlan"
	VLANID        int      `json:"vlan"`
	VLANType      int      `json:"vlanType"`
	Application   int      `json:"application"`
	GatewaySubnet string   `json:"gatewaySubnet"`
	InterfaceIDs  []string `json:"interfaceIds"`

	Isolation         bool `json:"isolation"`
	AllLan            bool `json:"allLan"`
	Portal            bool `json:"portal"`
	RateLimit         bool `json:"rateLimit"`
	QosQueueEnable    bool `json:"qosQueueEnable"`
	AccessControlRule bool `json:"accessControlRule"`
	ArpDetection      bool `json:"arpDetectionEnable"`
	IGMPSnoop         bool `json:"igmpSnoopEnable"`
	FastLeave         bool `json:"fastLeaveEnable"`
	MLDSnoop          bool `json:"mldSnoopEnable"`
	DHCPL2Relay       bool `json:"dhcpL2RelayEnable"`

	DHCPGuard    EnableFlag   `json:"dhcpGuard"`
	DHCPv6Guard  EnableFlag   `json:"dhcpv6Guard"`
	IPv6Config   IntEnable    `json:"lanNetworkIpv6Config"`
	DHCPSettings DHCPSettings `json:"dhcpSettings"`
}

// DHCPEnabled reports whether the DHCP server is enabled on this network.
func (n Network) DHCPEnabled() bool { return n.DHCPSettings.Enable }

func networksPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/lan/networks", siteID)
}

// CreateNetwork creates a LAN network.
//
// NOTE: creating a brand-new "interface" network is not supported by this
// endpoint on v6.2 controllers (the UI uses the Omada OpenAPI). See the README.
// CreateNetwork creates a LAN network through the Open API.
//
// Create is the one network operation the web API will not do — POSTing to
// /setting/lan/networks is rejected outright. It lives on the Open API instead:
//
//	POST /openapi/v2/{omadacId}/sites/{site}/lan-networks
//
// (v1 exists too but demands a longer required-field list for no benefit.)
//
// **Only the four fields the endpoint requires are sent here**, and everything
// else the practitioner configured is applied straight afterwards by the
// ordinary web-API update. That split is deliberate. The two surfaces describe
// a network differently — the Open API nests DHCP under `dhcpSettingsVO`, the
// web API uses `dhcpSettings` — so translating the full configuration into Open
// API shape would mean maintaining a second, largely untested mapping of every
// field, and a mistake in it would land on a live VLAN. Creating a minimal
// network and then updating it through the code path that is already exercised
// on every apply is both less code and better tested.
//
// The consequence worth knowing: creating a network is two calls, so an
// interruption between them can leave a network that exists but is not yet
// fully configured. It will be reconciled on the next apply.
func (c *Client) CreateNetwork(ctx context.Context, siteID string, fields map[string]any) (*Network, error) {
	if !c.openAPIConfigured() {
		return nil, fmt.Errorf("creating a network needs Open API credentials: %w", ErrOpenAPINotConfigured)
	}

	name, _ := fields["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("creating network: name is required")
	}

	seed := map[string]any{
		"name":            name,
		"purpose":         valueOr(fields, "purpose", 1),
		"vlan":            fields["vlan"],
		"igmpSnoopEnable": valueOr(fields, "igmpSnoopEnable", false),
		"interfaceIds":    fields["interfaceIds"],
	}
	if seed["vlan"] == nil {
		return nil, fmt.Errorf("creating network %q: vlan is required", name)
	}
	// The controller rejects a network bound to nothing (-33515 "LAN interfaces
	// could not be none"), so this cannot be deferred to the update call the way
	// the rest of the configuration is. It is checked here to give a better
	// message than the raw code, and because the check is cheap.
	if ifaces, ok := seed["interfaceIds"].([]string); !ok || len(ifaces) == 0 {
		return nil, fmt.Errorf("creating network %q: interface_ids must list at least one LAN "+
			"interface — the controller refuses a network that is not bound to one (-33515). "+
			"Copy the ids from an existing network via the omada_networks data source", name)
	}

	path := c.OpenAPIPathVersion(2, siteID, "/lan-networks")
	if err := c.DoOpenAPI(ctx, "POST", path, seed, nil); err != nil {
		return nil, fmt.Errorf("creating network %q: %w", name, err)
	}

	// The create response carries no id, so the new network is located by name.
	created, err := c.getNetworkByName(ctx, siteID, name)
	if err != nil {
		return nil, fmt.Errorf("network %q was created but could not be read back: %w", name, err)
	}

	// Apply the rest of the configuration on the surface that supports it.
	rest := map[string]any{}
	for k, v := range fields {
		if _, seeded := seed[k]; !seeded {
			rest[k] = v
		}
	}
	if len(rest) == 0 {
		return created, nil
	}
	updated, err := c.UpdateNetwork(ctx, siteID, created.ID, rest)
	if err != nil {
		return nil, fmt.Errorf("network %q was created but its configuration could not be applied "+
			"(it exists on the controller and will be reconciled on the next apply): %w", name, err)
	}
	return updated, nil
}

// valueOr returns fields[key], or def when it is absent or nil.
func valueOr(fields map[string]any, key string, def any) any {
	if v, ok := fields[key]; ok && v != nil {
		return v
	}
	return def
}

// GetNetwork returns a single network by id. The controller has no single-object
// GET for LAN networks, so this filters the (validated) list endpoint.
func (c *Client) GetNetwork(ctx context.Context, siteID, id string) (*Network, error) {
	nets, err := c.ListNetworks(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range nets {
		if nets[i].ID == id {
			return &nets[i], nil
		}
	}
	return nil, fmt.Errorf("network %q not found on site %q", id, siteID)
}

func (c *Client) getNetworkByName(ctx context.Context, siteID, name string) (*Network, error) {
	nets, err := c.ListNetworks(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range nets {
		if nets[i].Name == name {
			return &nets[i], nil
		}
	}
	return nil, fmt.Errorf("network %q not found on site %q after create", name, siteID)
}

// UpdateNetwork does a read-modify-write: fetch the current object, overlay the
// managed fields, and PATCH the whole thing back so derived/unmodelled fields
// (ipRangePool, totalIpNum, ...) survive.
func (c *Client) UpdateNetwork(ctx context.Context, siteID, id string, fields map[string]any) (*Network, error) {
	cur, err := c.RawByID(ctx, networksPath(siteID), "id", id)
	if err != nil {
		return nil, err
	}
	for k, v := range fields {
		if k == "dhcpSettings" {
			// merge into the existing dhcpSettings so ipRangePool etc. survive
			base, _ := cur["dhcpSettings"].(map[string]any)
			merged := map[string]any{}
			for bk, bv := range base {
				merged[bk] = bv
			}
			for nk, nv := range v.(map[string]any) {
				merged[nk] = nv
			}
			cur["dhcpSettings"] = merged
			continue
		}
		cur[k] = v
	}
	if err := c.Do(ctx, "PATCH", networksPath(siteID)+"/"+id, cur, nil); err != nil {
		return nil, fmt.Errorf("updating network %q: %w", id, err)
	}
	return c.GetNetwork(ctx, siteID, id)
}

// DeleteNetwork removes a network.
func (c *Client) DeleteNetwork(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", networksPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting network %q: %w", id, err)
	}
	return nil
}

// ListNetworks returns every LAN network for the given site, following pagination.
func (c *Client) ListNetworks(ctx context.Context, siteID string) ([]Network, error) {
	return listAll[Network](ctx, c, "networks", networksPath(siteID))
}
