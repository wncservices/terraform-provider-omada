// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// ipGroupType is the "IP group" profile type (0). Domain (7) and IPv6 (3) groups
// are separate types not yet modelled.
const ipGroupType = 0

// IPGroupEntry is one CIDR entry in an IP group.
type IPGroupEntry struct {
	IP          string `json:"ip"`
	Mask        int    `json:"mask"`
	Description string `json:"description"`
}

// IPGroup is a reusable IP group (used by firewall ACLs etc.). Verified against
// a live v6.2 controller (/setting/profiles/groups). The ID field is groupId.
type IPGroup struct {
	GroupID string         `json:"groupId"`
	Name    string         `json:"name"`
	Type    int            `json:"type"`
	IPList  []IPGroupEntry `json:"ipList"`
}

// IPGroupInput is the create/update payload.
type IPGroupInput struct {
	GroupID string         `json:"groupId,omitempty"`
	Name    string         `json:"name"`
	Type    int            `json:"type"`
	IPList  []IPGroupEntry `json:"ipList"`
}

func groupsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/profiles/groups", siteID)
}

// ListIPGroups returns all IP-type groups for a site.
func (c *Client) ListIPGroups(ctx context.Context, siteID string) ([]IPGroup, error) {
	all, err := listAll[IPGroup](ctx, c, "ip groups", groupsPath(siteID))
	if err != nil {
		return nil, err
	}
	groups := make([]IPGroup, 0, len(all))
	for _, g := range all {
		if g.Type == ipGroupType {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

// GetIPGroup returns a single IP group by id.
func (c *Client) GetIPGroup(ctx context.Context, siteID, groupID string) (*IPGroup, error) {
	groups, err := c.ListIPGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].GroupID == groupID {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("ip group %q not found on site %q", groupID, siteID)
}

// CreateIPGroup creates a group (null result → resolved by name).
func (c *Client) CreateIPGroup(ctx context.Context, siteID string, in *IPGroupInput) (*IPGroup, error) {
	in.Type = ipGroupType
	if err := c.Do(ctx, "POST", groupsPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating ip group: %w", err)
	}
	groups, err := c.ListIPGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Name == in.Name {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("ip group %q not found after create", in.Name)
}

// UpdateIPGroup patches a group. The item path is /groups/{type}/{groupId}.
func (c *Client) UpdateIPGroup(ctx context.Context, siteID, groupID string, in *IPGroupInput) (*IPGroup, error) {
	in.Type = ipGroupType
	in.GroupID = groupID
	path := fmt.Sprintf("%s/%d/%s", groupsPath(siteID), ipGroupType, groupID)
	if err := c.Do(ctx, "PATCH", path, in, nil); err != nil {
		return nil, fmt.Errorf("updating ip group %q: %w", groupID, err)
	}
	return c.GetIPGroup(ctx, siteID, groupID)
}

// DeleteIPGroup removes a group.
func (c *Client) DeleteIPGroup(ctx context.Context, siteID, groupID string) error {
	path := fmt.Sprintf("%s/%d/%s", groupsPath(siteID), ipGroupType, groupID)
	if err := c.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("deleting ip group %q: %w", groupID, err)
	}
	return nil
}
