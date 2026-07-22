// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// WLANGroup is a WLAN group (a set of SSIDs). Verified against a live v6.2
// controller (/setting/wlans). Create needs clone:false; update uses PATCH.
type WLANGroup struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
}

type wlanGroupInput struct {
	Name  string `json:"name"`
	Clone bool   `json:"clone"`
}

func wlansPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/wlans", siteID)
}

// ListWLANGroups returns all WLAN groups for a site.
func (c *Client) ListWLANGroups(ctx context.Context, siteID string) ([]WLANGroup, error) {
	var out struct {
		Data []WLANGroup `json:"data"`
	}
	if err := c.Do(ctx, "GET", wlansPath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing wlan groups: %w", err)
	}
	if len(out.Data) == 0 {
		// Some controllers return the list unwrapped.
		var bare []WLANGroup
		if err := c.Do(ctx, "GET", wlansPath(siteID), nil, &bare); err == nil {
			return bare, nil
		}
	}
	return out.Data, nil
}

// GetWLANGroup returns a WLAN group by id.
func (c *Client) GetWLANGroup(ctx context.Context, siteID, id string) (*WLANGroup, error) {
	groups, err := c.ListWLANGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == id {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("wlan group %q not found on site %q", id, siteID)
}

// CreateWLANGroup creates a group (null result → resolved by name).
func (c *Client) CreateWLANGroup(ctx context.Context, siteID, name string) (*WLANGroup, error) {
	if err := c.Do(ctx, "POST", wlansPath(siteID), wlanGroupInput{Name: name, Clone: false}, nil); err != nil {
		return nil, fmt.Errorf("creating wlan group: %w", err)
	}
	groups, err := c.ListWLANGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Name == name {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("wlan group %q not found after create", name)
}

// UpdateWLANGroup renames a group (PATCH).
func (c *Client) UpdateWLANGroup(ctx context.Context, siteID, id, name string) (*WLANGroup, error) {
	if err := c.Do(ctx, "PATCH", wlansPath(siteID)+"/"+id, wlanGroupInput{Name: name, Clone: false}, nil); err != nil {
		return nil, fmt.Errorf("updating wlan group %q: %w", id, err)
	}
	return c.GetWLANGroup(ctx, siteID, id)
}

// DeleteWLANGroup removes a group.
func (c *Client) DeleteWLANGroup(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", wlansPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting wlan group %q: %w", id, err)
	}
	return nil
}
