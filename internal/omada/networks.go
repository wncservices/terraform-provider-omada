// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
)

// Network is a LAN network (VLAN interface) on a site. Field mappings verified
// against a live v6.2 controller (GET /sites/{id}/setting/lan/networks).
type Network struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Purpose       string       `json:"purpose"`       // "interface" | "vlan"
	VLANID        int          `json:"vlan"`          // controller field is "vlan"
	GatewaySubnet string       `json:"gatewaySubnet"` // CIDR, e.g. 10.10.99.1/24
	InterfaceIDs  []string     `json:"interfaceIds"`  // gateway LAN interfaces bound to an "interface" network
	DHCPSettings  DHCPSettings `json:"dhcpSettings"`
}

// DHCPSettings is the nested DHCP server config. Note: this is distinct from the
// controller's "dhcpGuard" object, which is a separate rogue-DHCP protection.
type DHCPSettings struct {
	Enable       bool   `json:"enable"`
	IPAddrStart  string `json:"ipaddrStart"`
	IPAddrEnd    string `json:"ipaddrEnd"`
	LeaseTimeMin int    `json:"leasetime"`
}

// DHCPEnabled reports whether the DHCP server is enabled on this network.
func (n Network) DHCPEnabled() bool { return n.DHCPSettings.Enable }

// NetworkInput is the payload sent when creating or updating a network. Only
// the fields we manage are sent; the controller fills the rest with defaults.
type NetworkInput struct {
	Name          string     `json:"name"`
	Purpose       string     `json:"purpose"`
	VLANID        int        `json:"vlan"`
	GatewaySubnet string     `json:"gatewaySubnet,omitempty"`
	InterfaceIDs  []string   `json:"interfaceIds,omitempty"`
	DHCPSettings  *DHCPInput `json:"dhcpSettings,omitempty"`
	IGMPSnoop     bool       `json:"igmpSnoopEnable"`
}

// DHCPInput is the DHCP-server portion of a NetworkInput.
type DHCPInput struct {
	Enable      bool   `json:"enable"`
	IPAddrStart string `json:"ipaddrStart,omitempty"`
	IPAddrEnd   string `json:"ipaddrEnd,omitempty"`
}

func networksPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/lan/networks", siteID)
}

// CreateNetwork creates a LAN network and returns the created object.
func (c *Client) CreateNetwork(ctx context.Context, siteID string, in *NetworkInput) (*Network, error) {
	var created Network
	if err := c.Do(ctx, "POST", networksPath(siteID), in, &created); err != nil {
		return nil, fmt.Errorf("creating network: %w", err)
	}
	// Some controller versions return only an id on create; fall back to a
	// lookup by name so callers always get a fully-populated object.
	if created.ID == "" {
		return c.getNetworkByName(ctx, siteID, in.Name)
	}
	return &created, nil
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

// UpdateNetwork patches an existing network.
func (c *Client) UpdateNetwork(ctx context.Context, siteID, id string, in *NetworkInput) (*Network, error) {
	if err := c.Do(ctx, "PATCH", networksPath(siteID)+"/"+id, in, nil); err != nil {
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
	var all []Network
	page := 1
	const size = 100

	for {
		var pr PaginatedResult
		path := fmt.Sprintf("/sites/%s/setting/lan/networks?currentPage=%d&currentPageSize=%d", siteID, page, size)
		if err := c.Do(ctx, "GET", path, nil, &pr); err != nil {
			return nil, fmt.Errorf("listing networks: %w", err)
		}

		var chunk []Network
		if len(pr.Data) > 0 {
			if err := json.Unmarshal(pr.Data, &chunk); err != nil {
				return nil, fmt.Errorf("decoding networks page %d: %w", page, err)
			}
		}
		all = append(all, chunk...)

		if len(all) >= pr.TotalRows || len(chunk) == 0 {
			break
		}
		page++
	}
	return all, nil
}
