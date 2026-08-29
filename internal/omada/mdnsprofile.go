// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// MDNSProfile is a custom mDNS service definition (Settings -> Services ->
// mDNS -> "Custom Service" when editing a reflector rule): a name bundling
// one or more raw mDNS service strings (e.g. "_matter._tcp.local").
// Referenced by id from MDNSAPCfg.ProfileIDs, alongside the controller's
// read-only built-ins ("buildIn-1" .. "buildIn-10").
//
// The read shape and update verb are verified against a live v6.2 controller
// (/setting/profiles/mdns, distinct from the reflector-rule collection at
// /setting/service/mdns); update is PUT. Create was NOT verified before
// shipping — it was assumed to answer a bare id string, matching
// ServiceType. It doesn't: the real controller answers the created object,
// and the wrong decode target left an orphaned profile on a live site (the
// POST succeeded server-side; the client crashed decoding the response, so
// Terraform never recorded it and retried on the next apply, hitting -33756
// "already exists"). See CreateMDNSProfile.
//
// The controller caps the number of CUSTOM profiles per site — 5 on the
// hardware this was developed against, reported back as
// mdnsCustomMaxProfileNum alongside the list and not otherwise exposed here.
// Exceeding it fails at create time with the controller's own error.
type MDNSProfile struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	ServiceIDs []string `json:"serviceId"`
	// Default marks a controller built-in ("buildIn-N"). Read-only.
	Default bool `json:"defaultProfile,omitempty"`
}

func mdnsProfilePath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/profiles/mdns", siteID)
}

// ListMDNSProfiles returns every mDNS service profile, built-ins included.
func (c *Client) ListMDNSProfiles(ctx context.Context, siteID string) ([]MDNSProfile, error) {
	return listAll[MDNSProfile](ctx, c, "mdns profiles", mdnsProfilePath(siteID))
}

// GetMDNSProfile returns one mDNS profile by id.
func (c *Client) GetMDNSProfile(ctx context.Context, siteID, id string) (*MDNSProfile, error) {
	items, err := c.ListMDNSProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("mdns profile %q not found", id)
}

// CreateMDNSProfile creates a custom mDNS profile and returns its id.
//
// Unlike ServiceType, the response here is the created object, not a bare id
// string (see the MDNSProfile doc comment for how that was found out). Fall
// back to resolving by name regardless, the same as a null/empty response —
// cheap insurance against this decoding correctly but some other field
// mismatch leaving ID empty.
func (c *Client) CreateMDNSProfile(ctx context.Context, siteID string, p MDNSProfile) (string, error) {
	p.ID = ""
	p.Default = false
	var created MDNSProfile
	if err := c.Do(ctx, http.MethodPost, mdnsProfilePath(siteID), p, &created); err != nil {
		return "", fmt.Errorf("creating mdns profile: %w", err)
	}
	if created.ID != "" {
		return created.ID, nil
	}
	items, err := c.ListMDNSProfiles(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == p.Name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created mdns profile %q but could not resolve its id", p.Name)
}

// UpdateMDNSProfile updates a custom mDNS profile. Note PUT, matching ServiceType.
func (c *Client) UpdateMDNSProfile(ctx context.Context, siteID, id string, p MDNSProfile) error {
	p.ID = id
	p.Default = false
	path := fmt.Sprintf("%s/%s", mdnsProfilePath(siteID), id)
	if err := c.Do(ctx, http.MethodPut, path, p, nil); err != nil {
		return fmt.Errorf("updating mdns profile: %w", err)
	}
	return nil
}

// DeleteMDNSProfile removes a custom mDNS profile.
func (c *Client) DeleteMDNSProfile(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", mdnsProfilePath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting mdns profile: %w", err)
	}
	return nil
}
