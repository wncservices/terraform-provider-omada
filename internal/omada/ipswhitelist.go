// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// IPS whitelist entries exempt a traffic source from intrusion prevention
// (Settings -> Network Security -> IPS/IDS -> Allow List).
//
// The read and write paths differ, which is easy to trip over: the list is
// served from a **`/grid/`** view that exists for the UI table and rejects
// anything but GET, while create and delete live one level up.
//
//	GET    /setting/ips/grid/whitelist   list (paginated)
//	POST   /setting/ips/whitelist        create
//	DELETE /setting/ips/whitelist/{id}   delete
//
// There is no update verb, and nothing to update: an entry is only
// `direction` + `trafficType` + `trafficSource`, all of which are the rule
// itself. Changing any of them is a different rule, so the resource replaces.

// IPSWhitelistEntry exempts one traffic source from IPS inspection.
type IPSWhitelistEntry struct {
	ID string `json:"id,omitempty"`
	// Direction is a controller enum for which way the traffic flows.
	Direction int `json:"direction"`
	// TrafficType selects what TrafficSource identifies; `1` pairs with a
	// network id.
	TrafficType int `json:"trafficType"`
	// TrafficSource is the id of the exempted object — a network id when
	// TrafficType is 1.
	TrafficSource string `json:"trafficSource"`
}

func ipsWhitelistListPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/ips/grid/whitelist", siteID)
}

func ipsWhitelistItemPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/ips/whitelist", siteID)
}

// ListIPSWhitelist returns every whitelist entry on the site.
func (c *Client) ListIPSWhitelist(ctx context.Context, siteID string) ([]IPSWhitelistEntry, error) {
	return listAll[IPSWhitelistEntry](ctx, c, "ips whitelist entries", ipsWhitelistListPath(siteID))
}

// GetIPSWhitelistEntry returns one entry by id.
func (c *Client) GetIPSWhitelistEntry(ctx context.Context, siteID, id string) (*IPSWhitelistEntry, error) {
	items, err := c.ListIPSWhitelist(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("ips whitelist entry %q not found", id)
}

// CreateIPSWhitelistEntry adds an entry and returns its id. The controller
// answers with a null result, so the new entry is resolved from the list by
// matching its three defining fields.
func (c *Client) CreateIPSWhitelistEntry(ctx context.Context, siteID string, e IPSWhitelistEntry) (string, error) {
	e.ID = ""
	if err := c.Do(ctx, http.MethodPost, ipsWhitelistItemPath(siteID), e, nil); err != nil {
		return "", fmt.Errorf("creating ips whitelist entry: %w", err)
	}
	items, err := c.ListIPSWhitelist(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Direction == e.Direction && it.TrafficType == e.TrafficType && it.TrafficSource == e.TrafficSource {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created ips whitelist entry but could not resolve its id")
}

// DeleteIPSWhitelistEntry removes an entry.
func (c *Client) DeleteIPSWhitelistEntry(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", ipsWhitelistItemPath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting ips whitelist entry: %w", err)
	}
	return nil
}
