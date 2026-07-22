// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// PortProfile is a switch port profile. Verified against a live v6.2 controller
// (/setting/lan/profiles). This provider manages a subset of fields; the rest
// are preserved on update (read-modify-write). Create returns null; update
// uses PATCH.
type PortProfile struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	POE              int      `json:"poe"` // 0=off, 1=on(?), 2=keep
	NativeNetworkID  string   `json:"nativeNetworkId"`
	TagNetworkIDs    []string `json:"tagNetworkIds"`
	UntagNetworkIDs  []string `json:"untagNetworkIds"`
	VLANConfigEnable bool     `json:"vlanConfigEnable"`
}

func profilesPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/lan/profiles", siteID)
}

func profilesListPath(siteID string) string {
	return profilesPath(siteID) + "?currentPage=1&currentPageSize=1000"
}

// ListPortProfiles returns all port profiles for a site.
func (c *Client) ListPortProfiles(ctx context.Context, siteID string) ([]PortProfile, error) {
	var out struct {
		Data []PortProfile `json:"data"`
	}
	if err := c.Do(ctx, "GET", profilesListPath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing port profiles: %w", err)
	}
	return out.Data, nil
}

// GetPortProfile returns one profile by id.
func (c *Client) GetPortProfile(ctx context.Context, siteID, id string) (*PortProfile, error) {
	profs, err := c.ListPortProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range profs {
		if profs[i].ID == id {
			return &profs[i], nil
		}
	}
	return nil, fmt.Errorf("port profile %q not found on site %q", id, siteID)
}

// CreatePortProfile creates a profile from the given fields (null result →
// resolved by name).
func (c *Client) CreatePortProfile(ctx context.Context, siteID string, fields map[string]any) (*PortProfile, error) {
	name, _ := fields["name"].(string)
	if err := c.Do(ctx, "POST", profilesPath(siteID), fields, nil); err != nil {
		return nil, fmt.Errorf("creating port profile: %w", err)
	}
	profs, err := c.ListPortProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range profs {
		if profs[i].Name == name {
			return &profs[i], nil
		}
	}
	return nil, fmt.Errorf("port profile %q not found after create", name)
}

// UpdatePortProfile does a read-modify-write: it fetches the current object,
// overlays the managed fields, and PATCHes the whole thing back so unmanaged
// fields are preserved.
func (c *Client) UpdatePortProfile(ctx context.Context, siteID, id string, fields map[string]any) (*PortProfile, error) {
	cur, err := c.RawByID(ctx, profilesListPath(siteID), "id", id)
	if err != nil {
		return nil, err
	}
	for k, v := range fields {
		cur[k] = v
	}
	if err := c.Do(ctx, "PATCH", profilesPath(siteID)+"/"+id, cur, nil); err != nil {
		return nil, fmt.Errorf("updating port profile %q: %w", id, err)
	}
	return c.GetPortProfile(ctx, siteID, id)
}

// DeletePortProfile removes a profile.
func (c *Client) DeletePortProfile(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", profilesPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting port profile %q: %w", id, err)
	}
	return nil
}
