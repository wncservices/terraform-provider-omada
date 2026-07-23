// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// LanDNS is a local ("LAN") DNS record. Field mappings verified against a live
// v6.2 controller (GET /sites/{id}/setting/lan/dns).
type LanDNS struct {
	ID            string   `json:"id"`
	Enable        bool     `json:"enable"`
	Name          string   `json:"name"`
	Domain        string   `json:"domain"`
	Type          int      `json:"type"`
	Aliases       []string `json:"aliases"`
	IPAddresses   []string `json:"ipAddresses"`
	IPv6Addresses []string `json:"ipv6Addresses"`
	LanNetworkIDs []string `json:"lanNetworkIds"` // must reference existing networks
	CustomTTL     bool     `json:"customTtl"`
}

// LanDNSInput is the create/update payload.
type LanDNSInput struct {
	Enable        bool     `json:"enable"`
	Name          string   `json:"name"`
	Domain        string   `json:"domain"`
	Type          int      `json:"type"`
	Aliases       []string `json:"aliases"`
	IPAddresses   []string `json:"ipAddresses"`
	IPv6Addresses []string `json:"ipv6Addresses"`
	LanNetworkIDs []string `json:"lanNetworkIds"`
	CustomTTL     bool     `json:"customTtl"`
}

func lanDNSPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/lan/dns", siteID)
}

// ListLanDNS returns all LAN DNS records for a site.
func (c *Client) ListLanDNS(ctx context.Context, siteID string) ([]LanDNS, error) {
	return listAll[LanDNS](ctx, c, "lan dns", lanDNSPath(siteID))
}

// GetLanDNS returns a single LAN DNS record by id (filters the list endpoint).
func (c *Client) GetLanDNS(ctx context.Context, siteID, id string) (*LanDNS, error) {
	recs, err := c.ListLanDNS(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if recs[i].ID == id {
			return &recs[i], nil
		}
	}
	return nil, fmt.Errorf("lan dns %q not found on site %q", id, siteID)
}

// CreateLanDNS creates a record. The controller returns an empty result on
// create, so the new record is resolved by name.
func (c *Client) CreateLanDNS(ctx context.Context, siteID string, in *LanDNSInput) (*LanDNS, error) {
	if err := c.Do(ctx, "POST", lanDNSPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating lan dns: %w", err)
	}
	recs, err := c.ListLanDNS(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range recs {
		if recs[i].Name == in.Name {
			return &recs[i], nil
		}
	}
	return nil, fmt.Errorf("lan dns %q not found after create", in.Name)
}

// UpdateLanDNS patches a record.
func (c *Client) UpdateLanDNS(ctx context.Context, siteID, id string, in *LanDNSInput) (*LanDNS, error) {
	if err := c.Do(ctx, "PATCH", lanDNSPath(siteID)+"/"+id, in, nil); err != nil {
		return nil, fmt.Errorf("updating lan dns %q: %w", id, err)
	}
	return c.GetLanDNS(ctx, siteID, id)
}

// DeleteLanDNS removes a record.
func (c *Client) DeleteLanDNS(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", lanDNSPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting lan dns %q: %w", id, err)
	}
	return nil
}
