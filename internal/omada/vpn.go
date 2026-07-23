// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// VPN is a VPN server/policy. The read shape is verified against a live v6.2
// controller (/setting/vpns), but its write verbs are NOT live-validated — this
// provider manages a small subset (name/enable) via read-modify-write so the
// full config is preserved, and infers PUT for updates. Use with care and
// prefer import + toggling over create/delete.
type VPN struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  bool   `json:"status"`
	Purpose int    `json:"purpose"`
}

func vpnsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/vpns", siteID)
}

// ListVPNs returns all VPNs for a site.
func (c *Client) ListVPNs(ctx context.Context, siteID string) ([]VPN, error) {
	return listAll[VPN](ctx, c, "vpns", vpnsPath(siteID))
}

// GetVPN returns one VPN by id.
func (c *Client) GetVPN(ctx context.Context, siteID, id string) (*VPN, error) {
	vpns, err := c.ListVPNs(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range vpns {
		if vpns[i].ID == id {
			return &vpns[i], nil
		}
	}
	return nil, fmt.Errorf("vpn %q not found on site %q", id, siteID)
}

// CreateVPN creates a VPN from the given fields (null result → resolved by name).
func (c *Client) CreateVPN(ctx context.Context, siteID string, fields map[string]any) (*VPN, error) {
	name, _ := fields["name"].(string)
	if err := c.Do(ctx, "POST", vpnsPath(siteID), fields, nil); err != nil {
		return nil, fmt.Errorf("creating vpn: %w", err)
	}
	vpns, err := c.ListVPNs(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range vpns {
		if vpns[i].Name == name {
			return &vpns[i], nil
		}
	}
	return nil, fmt.Errorf("vpn %q not found after create", name)
}

// UpdateVPN does a read-modify-write of the managed fields (PUT).
func (c *Client) UpdateVPN(ctx context.Context, siteID, id string, fields map[string]any) (*VPN, error) {
	cur, err := c.RawByID(ctx, vpnsPath(siteID), "id", id)
	if err != nil {
		return nil, err
	}
	for k, v := range fields {
		cur[k] = v
	}
	if err := c.Do(ctx, "PUT", vpnsPath(siteID)+"/"+id, cur, nil); err != nil {
		return nil, fmt.Errorf("updating vpn %q: %w", id, err)
	}
	return c.GetVPN(ctx, siteID, id)
}

// DeleteVPN removes a VPN.
func (c *Client) DeleteVPN(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", vpnsPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting vpn %q: %w", id, err)
	}
	return nil
}
