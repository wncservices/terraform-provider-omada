// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// portGroupType is the "port group" profile type (1), alongside IP groups (0),
// IPv6 (3) and domain (7) groups on the same /setting/profiles/groups endpoint.
const portGroupType = 1

// PortGroup is a reusable group of L4 ports (used by firewall ACLs via
// source_type/destination_type = 2). Verified against a live v6.2 controller
// (/setting/profiles/groups, type=1). The ID field is groupId; entries in
// PortList are single ports ("8080") or ranges ("9000-9100").
type PortGroup struct {
	GroupID      string   `json:"groupId"`
	Name         string   `json:"name"`
	Type         int      `json:"type"`
	PortType     int      `json:"portType"`
	PortList     []string `json:"portList"`
	PortMaskList []any    `json:"portMaskList"`
}

// PortGroupInput is the create/update payload. ipList is sent empty — the
// controller rejects a type-1 group create without the (empty) ipList field.
type PortGroupInput struct {
	GroupID      string   `json:"groupId,omitempty"`
	Name         string   `json:"name"`
	Type         int      `json:"type"`
	IPList       []any    `json:"ipList"`
	PortType     int      `json:"portType"`
	PortList     []string `json:"portList"`
	PortMaskList []any    `json:"portMaskList"`
}

// ListPortGroups returns all port-type groups for a site.
func (c *Client) ListPortGroups(ctx context.Context, siteID string) ([]PortGroup, error) {
	all, err := listAll[PortGroup](ctx, c, "port groups", groupsPath(siteID))
	if err != nil {
		return nil, err
	}
	groups := make([]PortGroup, 0, len(all))
	for _, g := range all {
		if g.Type == portGroupType {
			groups = append(groups, g)
		}
	}
	return groups, nil
}

// GetPortGroup returns a single port group by id.
func (c *Client) GetPortGroup(ctx context.Context, siteID, groupID string) (*PortGroup, error) {
	groups, err := c.ListPortGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].GroupID == groupID {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("port group %q not found on site %q", groupID, siteID)
}

// CreatePortGroup creates a group (null result → resolved by name).
func (c *Client) CreatePortGroup(ctx context.Context, siteID string, in *PortGroupInput) (*PortGroup, error) {
	in.Type = portGroupType
	if in.IPList == nil {
		in.IPList = []any{}
	}
	if in.PortMaskList == nil {
		in.PortMaskList = []any{}
	}
	if in.PortList == nil {
		in.PortList = []string{}
	}
	if err := c.Do(ctx, "POST", groupsPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating port group: %w", err)
	}
	groups, err := c.ListPortGroups(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Name == in.Name {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("port group %q not found after create", in.Name)
}

// UpdatePortGroup patches a group. The item path is /groups/{type}/{groupId}.
func (c *Client) UpdatePortGroup(ctx context.Context, siteID, groupID string, in *PortGroupInput) (*PortGroup, error) {
	in.Type = portGroupType
	in.GroupID = groupID
	if in.IPList == nil {
		in.IPList = []any{}
	}
	if in.PortMaskList == nil {
		in.PortMaskList = []any{}
	}
	if in.PortList == nil {
		in.PortList = []string{}
	}
	path := fmt.Sprintf("%s/%d/%s", groupsPath(siteID), portGroupType, groupID)
	if err := c.Do(ctx, "PATCH", path, in, nil); err != nil {
		return nil, fmt.Errorf("updating port group %q: %w", groupID, err)
	}
	return c.GetPortGroup(ctx, siteID, groupID)
}

// DeletePortGroup removes a group.
func (c *Client) DeletePortGroup(ctx context.Context, siteID, groupID string) error {
	path := fmt.Sprintf("%s/%d/%s", groupsPath(siteID), portGroupType, groupID)
	if err := c.Do(ctx, "DELETE", path, nil, nil); err != nil {
		return fmt.Errorf("deleting port group %q: %w", groupID, err)
	}
	return nil
}
