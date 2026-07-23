// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// MDNSReflector is an mDNS reflection rule (bridges mDNS between VLANs).
// Verified against a live v6.2 controller (/setting/service/mdns). Create
// returns null; update uses PUT.
type MDNSReflector struct {
	ID     string    `json:"id"`
	Name   string    `json:"name"`
	Status bool      `json:"status"`
	Type   int       `json:"type"`
	AP     MDNSAPCfg `json:"ap"`
}

// MDNSAPCfg is the AP-based reflection config.
type MDNSAPCfg struct {
	ProfileIDs  []string `json:"profileIds"`
	ServiceVlan string   `json:"serviceVlan"`
	ClientVlan  string   `json:"clientVlan"`
}

// MDNSInput is the create/update payload.
type MDNSInput struct {
	ID     string    `json:"id,omitempty"`
	Name   string    `json:"name"`
	Status bool      `json:"status"`
	Type   int       `json:"type"`
	AP     MDNSAPCfg `json:"ap"`
}

func mdnsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/service/mdns", siteID)
}

// ListMDNS returns all mDNS rules for a site.
func (c *Client) ListMDNS(ctx context.Context, siteID string) ([]MDNSReflector, error) {
	return listAll[MDNSReflector](ctx, c, "mdns", mdnsPath(siteID))
}

// GetMDNS returns one mDNS rule by id.
func (c *Client) GetMDNS(ctx context.Context, siteID, id string) (*MDNSReflector, error) {
	rules, err := c.ListMDNS(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("mdns rule %q not found on site %q", id, siteID)
}

// CreateMDNS creates a rule (null result → resolved by name).
func (c *Client) CreateMDNS(ctx context.Context, siteID string, in *MDNSInput) (*MDNSReflector, error) {
	if err := c.Do(ctx, "POST", mdnsPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating mdns rule: %w", err)
	}
	rules, err := c.ListMDNS(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].Name == in.Name {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("mdns rule %q not found after create", in.Name)
}

// UpdateMDNS replaces a rule (PUT).
func (c *Client) UpdateMDNS(ctx context.Context, siteID, id string, in *MDNSInput) (*MDNSReflector, error) {
	in.ID = id
	if err := c.Do(ctx, "PUT", mdnsPath(siteID)+"/"+id, in, nil); err != nil {
		return nil, fmt.Errorf("updating mdns rule %q: %w", id, err)
	}
	return c.GetMDNS(ctx, siteID, id)
}

// DeleteMDNS removes a rule.
func (c *Client) DeleteMDNS(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", mdnsPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting mdns rule %q: %w", id, err)
	}
	return nil
}
