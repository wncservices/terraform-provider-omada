// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Client aliases (Clients -> client -> Config -> Alias) are persistent
// configuration attached to a client's MAC address. They are independent of
// DHCP reservations and remain available for known clients that are offline.
//
// Verified against a live v6.2 controller:
//
//	GET   /sites/{site}/clients/{mac}   full client document
//	PATCH /sites/{site}/clients/{mac}   partial update with {"name": "..."}
//
// PATCH is genuinely partial. Sending only name leaves fixed-address, rate
// limit, client-lock and controller-owned runtime fields untouched.
type ClientAlias struct {
	MAC  string `json:"mac"`
	Name string `json:"name"`
}

func clientAliasPath(siteID, mac string) string {
	return fmt.Sprintf("/sites/%s/clients/%s", siteID, NormalizeMAC(mac))
}

// GetClientAlias returns the persistent alias document fields for a known
// client. The controller's response includes runtime state, but only the two
// configuration identity fields are decoded here.
func (c *Client) GetClientAlias(ctx context.Context, siteID, mac string) (*ClientAlias, error) {
	if !ValidMAC(mac) {
		return nil, fmt.Errorf("reading client: invalid MAC address %q", mac)
	}
	var client ClientAlias
	if err := c.Do(ctx, http.MethodGet, clientAliasPath(siteID, mac), nil, &client); err != nil {
		return nil, fmt.Errorf("reading client %s: %w", mac, err)
	}
	client.MAC = NormalizeMAC(client.MAC)
	return &client, nil
}

// UpdateClientAlias changes only the display name for a known client.
func (c *Client) UpdateClientAlias(ctx context.Context, siteID, mac, alias string) error {
	if !ValidMAC(mac) {
		return fmt.Errorf("updating client alias: invalid MAC address %q", mac)
	}
	body := map[string]any{"name": alias}
	if err := c.Do(ctx, http.MethodPatch, clientAliasPath(siteID, mac), body, nil); err != nil {
		return fmt.Errorf("updating client alias for %s: %w", mac, err)
	}
	return nil
}

// ClearClientAlias restores the controller's unaliased representation by
// setting the display name to the client's normalized MAC address. A blank
// name is accepted but ignored by Omada 6.2, while this is the value the
// controller reports for clients without an alias.
func (c *Client) ClearClientAlias(ctx context.Context, siteID, mac string) error {
	return c.UpdateClientAlias(ctx, siteID, mac, NormalizeMAC(mac))
}
