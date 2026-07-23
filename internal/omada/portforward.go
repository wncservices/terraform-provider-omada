// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// PortForward is a NAT port-forwarding rule. Field mappings + endpoint verified
// against a live v6.2 controller (/setting/transmission/portForwardings).
// Note: create returns a null result (resolve by name); update uses PUT.
type PortForward struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        bool     `json:"status"` // enabled
	From          int      `json:"from"`
	WANPortIDs    []string `json:"interfaceWanPortId"`
	VirtualWANIDs []string `json:"virtualWanId"`
	ExternalPort  string   `json:"externalPort"`
	ForwardIP     string   `json:"forwardIp"`
	ForwardPort   string   `json:"forwardPort"`
	Protocol      int      `json:"protocol"` // 1=TCP, 2=UDP, 3=TCP+UDP
	DMZ           bool     `json:"dMZ"`
}

// PortForwardInput is the create/update payload. ID is set for PUT updates.
type PortForwardInput struct {
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name"`
	Status        bool     `json:"status"`
	From          int      `json:"from"`
	WANPortIDs    []string `json:"interfaceWanPortId"`
	VirtualWANIDs []string `json:"virtualWanId"`
	ExternalPort  string   `json:"externalPort"`
	ForwardIP     string   `json:"forwardIp"`
	ForwardPort   string   `json:"forwardPort"`
	Protocol      int      `json:"protocol"`
	DMZ           bool     `json:"dMZ"`
}

func portForwardPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/transmission/portForwardings", siteID)
}

// ListPortForwards returns all port-forwarding rules for a site.
func (c *Client) ListPortForwards(ctx context.Context, siteID string) ([]PortForward, error) {
	return listAll[PortForward](ctx, c, "port forwards", portForwardPath(siteID))
}

// GetPortForward returns a single rule by id.
func (c *Client) GetPortForward(ctx context.Context, siteID, id string) (*PortForward, error) {
	rules, err := c.ListPortForwards(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == id {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("port forward %q not found on site %q", id, siteID)
}

// DefaultWANPortIDs returns the WAN port binding used by existing rules, so a
// new rule can inherit it when the caller doesn't specify one.
func (c *Client) DefaultWANPortIDs(ctx context.Context, siteID string) ([]string, error) {
	rules, err := c.ListPortForwards(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if len(rules[i].WANPortIDs) > 0 {
			return rules[i].WANPortIDs, nil
		}
	}
	return nil, fmt.Errorf("no existing port-forward rule to infer the WAN port from; set wan_port_ids explicitly")
}

// CreatePortForward creates a rule (null result → resolved by name).
func (c *Client) CreatePortForward(ctx context.Context, siteID string, in *PortForwardInput) (*PortForward, error) {
	if err := c.Do(ctx, "POST", portForwardPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating port forward: %w", err)
	}
	rules, err := c.ListPortForwards(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].Name == in.Name {
			return &rules[i], nil
		}
	}
	return nil, fmt.Errorf("port forward %q not found after create", in.Name)
}

// UpdatePortForward replaces a rule (PUT — PATCH is unsupported here).
func (c *Client) UpdatePortForward(ctx context.Context, siteID, id string, in *PortForwardInput) (*PortForward, error) {
	in.ID = id
	if err := c.Do(ctx, "PUT", portForwardPath(siteID)+"/"+id, in, nil); err != nil {
		return nil, fmt.Errorf("updating port forward %q: %w", id, err)
	}
	return c.GetPortForward(ctx, siteID, id)
}

// DeletePortForward removes a rule.
func (c *Client) DeletePortForward(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", portForwardPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting port forward %q: %w", id, err)
	}
	return nil
}
