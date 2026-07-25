// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Disable-NAT rules (Settings -> Wired Networks -> Disable NAT) stop the
// gateway NATing traffic from the listed LANs out of a WAN interface, so those
// networks are routed rather than translated.
//
// The paths are asymmetric, which is easy to get wrong: the collection is
// **plural** (`disable-nats`) while create and the per-item path are
// **singular** (`disable-nat`, `disable-nat/{id}`). Update is `PUT`; `PATCH` is
// rejected with -1600.
//
// The controller allows **one rule per WAN port** (-34247 otherwise), so a site
// with a single WAN can hold exactly one of these.

// DisableNAT is one disable-NAT rule.
type DisableNAT struct {
	ID string `json:"id,omitempty"`
	// Name is the rule's display name.
	Name string `json:"name"`
	// Interface is the WAN interface id, in the controller's `1_<hex>` form.
	Interface string `json:"interface"`
	// LanList holds the ids of the LAN networks the rule applies to.
	LanList []string `json:"lanList"`
	// Status enables the rule. Enabling it stops NAT for those LANs.
	Status bool `json:"status"`
}

func disableNATListPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/wired-networks/disable-nats", siteID)
}

func disableNATItemPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/wired-networks/disable-nat", siteID)
}

// ListDisableNATs returns every disable-NAT rule on the site.
func (c *Client) ListDisableNATs(ctx context.Context, siteID string) ([]DisableNAT, error) {
	return listAll[DisableNAT](ctx, c, "disable-nat rules", disableNATListPath(siteID))
}

// GetDisableNAT returns one disable-NAT rule by id.
func (c *Client) GetDisableNAT(ctx context.Context, siteID, id string) (*DisableNAT, error) {
	items, err := c.ListDisableNATs(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("disable-nat rule %q not found", id)
}

// CreateDisableNAT creates a rule and returns its id. The controller does not
// echo the new object, so the id is resolved by name from the list.
func (c *Client) CreateDisableNAT(ctx context.Context, siteID string, d DisableNAT) (string, error) {
	d.ID = ""
	if d.LanList == nil {
		d.LanList = []string{}
	}
	if err := c.Do(ctx, http.MethodPost, disableNATItemPath(siteID), d, nil); err != nil {
		return "", fmt.Errorf("creating disable-nat rule: %w", err)
	}
	items, err := c.ListDisableNATs(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == d.Name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created disable-nat rule %q but could not resolve its id", d.Name)
}

// UpdateDisableNAT updates a rule in place. Note PUT — PATCH is rejected.
func (c *Client) UpdateDisableNAT(ctx context.Context, siteID, id string, d DisableNAT) error {
	d.ID = id
	if d.LanList == nil {
		d.LanList = []string{}
	}
	path := fmt.Sprintf("%s/%s", disableNATItemPath(siteID), id)
	if err := c.Do(ctx, http.MethodPut, path, d, nil); err != nil {
		return fmt.Errorf("updating disable-nat rule: %w", err)
	}
	return nil
}

// DeleteDisableNAT removes a rule.
func (c *Client) DeleteDisableNAT(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", disableNATItemPath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting disable-nat rule: %w", err)
	}
	return nil
}
