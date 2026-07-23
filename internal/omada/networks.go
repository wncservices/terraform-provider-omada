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
func (c *Client) CreateNetwork(ctx context.Context, siteID string, fields map[string]any) (*Network, error) {
	name, _ := fields["name"].(string)
	var created Network
	if err := c.Do(ctx, "POST", networksPath(siteID), fields, &created); err != nil {
		return nil, fmt.Errorf("creating network: %w", err)
	}
	if created.ID != "" {
		return &created, nil
	}
	return c.getNetworkByName(ctx, siteID, name)
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
