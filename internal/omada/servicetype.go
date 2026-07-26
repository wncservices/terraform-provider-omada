// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Service types (Settings -> Profiles -> Service Type) are reusable
// protocol/port definitions referenced by firewall and QoS rules.
//
// The controller ships twelve built-ins — ALL, FTP, SSH, TELNET, … — marked
// `defaultServiceType: true`. Those are read-only reference data: they can be
// listed and referenced but not modified, so this provider only manages custom
// ones. Update is `PUT` (`PATCH` → -1600).

// ServiceType is a protocol/port definition.
type ServiceType struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	// Protocol is a controller enum (0 = TCP on this firmware; `ALL` uses 2).
	Protocol    int    `json:"protocol"`
	SourcePorts string `json:"sourcePorts"`
	DestPorts   string `json:"destPorts"`
	Description string `json:"description,omitempty"`

	// Type and Code carry ICMP type/code where the protocol uses them.
	Type *int `json:"type,omitempty"`
	Code *int `json:"code,omitempty"`

	// Default marks a controller built-in. Read-only.
	Default bool `json:"defaultServiceType,omitempty"`
}

func serviceTypePath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/profiles/service-type", siteID)
}

// ListServiceTypes returns every service type, built-ins included.
func (c *Client) ListServiceTypes(ctx context.Context, siteID string) ([]ServiceType, error) {
	return listAll[ServiceType](ctx, c, "service types", serviceTypePath(siteID))
}

// GetServiceType returns one service type by id.
func (c *Client) GetServiceType(ctx context.Context, siteID, id string) (*ServiceType, error) {
	items, err := c.ListServiceTypes(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("service type %q not found", id)
}

// createIDResult covers endpoints that answer with the new id as a bare string.
type createIDResult = string

// CreateServiceType creates a custom service type and returns its id.
func (c *Client) CreateServiceType(ctx context.Context, siteID string, st ServiceType) (string, error) {
	st.ID = ""
	st.Default = false
	var id createIDResult
	if err := c.Do(ctx, http.MethodPost, serviceTypePath(siteID), st, &id); err != nil {
		return "", fmt.Errorf("creating service type: %w", err)
	}
	if id != "" {
		return id, nil
	}
	items, err := c.ListServiceTypes(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == st.Name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created service type %q but could not resolve its id", st.Name)
}

// UpdateServiceType updates a custom service type. Note PUT — PATCH is rejected.
func (c *Client) UpdateServiceType(ctx context.Context, siteID, id string, st ServiceType) error {
	st.ID = id
	st.Default = false
	path := fmt.Sprintf("%s/%s", serviceTypePath(siteID), id)
	if err := c.Do(ctx, http.MethodPut, path, st, nil); err != nil {
		return fmt.Errorf("updating service type: %w", err)
	}
	return nil
}

// DeleteServiceType removes a custom service type.
func (c *Client) DeleteServiceType(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", serviceTypePath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting service type: %w", err)
	}
	return nil
}
