// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Rate-limit profiles (Settings -> Profiles -> Rate Limit) cap per-client
// throughput and are referenced by SSIDs and portal authentication.
//
// The list is a bare JSON array. Create returns `{"id": "..."}`, update is
// `PATCH /{id}`, delete is `DELETE /{id}`.
//
// A limit value is only present in the document when its enable flag is set —
// the controller omits `downLimit` entirely while `downLimitEnable` is false —
// so those fields follow the "null is not false" invariant (§2.6).

// RateLimitProfile caps client throughput.
type RateLimitProfile struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`

	DownEnable bool `json:"downLimitEnable"`
	DownLimit  *int `json:"downLimit,omitempty"`
	UpEnable   bool `json:"upLimitEnable"`
	UpLimit    *int `json:"upLimit,omitempty"`

	// Default marks the controller's built-in profile. Read-only.
	Default bool `json:"isDefault,omitempty"`
}

func rateLimitPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/profiles/rateLimits", siteID)
}

// ListRateLimitProfiles returns every rate-limit profile on the site.
func (c *Client) ListRateLimitProfiles(ctx context.Context, siteID string) ([]RateLimitProfile, error) {
	var out []RateLimitProfile
	if err := c.Do(ctx, http.MethodGet, rateLimitPath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing rate limit profiles: %w", err)
	}
	return out, nil
}

// GetRateLimitProfile returns one profile by id.
func (c *Client) GetRateLimitProfile(ctx context.Context, siteID, id string) (*RateLimitProfile, error) {
	items, err := c.ListRateLimitProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("rate limit profile %q not found", id)
}

// CreateRateLimitProfile creates a profile and returns its id.
func (c *Client) CreateRateLimitProfile(ctx context.Context, siteID string, p RateLimitProfile) (string, error) {
	p.ID = ""
	p.Default = false
	var out struct {
		ID string `json:"id"`
	}
	if err := c.Do(ctx, http.MethodPost, rateLimitPath(siteID), p, &out); err != nil {
		return "", fmt.Errorf("creating rate limit profile: %w", err)
	}
	if out.ID != "" {
		return out.ID, nil
	}
	items, err := c.ListRateLimitProfiles(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == p.Name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created rate limit profile %q but could not resolve its id", p.Name)
}

// UpdateRateLimitProfile updates a profile in place.
func (c *Client) UpdateRateLimitProfile(ctx context.Context, siteID, id string, p RateLimitProfile) error {
	p.ID = id
	p.Default = false
	path := fmt.Sprintf("%s/%s", rateLimitPath(siteID), id)
	if err := c.Do(ctx, http.MethodPatch, path, p, nil); err != nil {
		return fmt.Errorf("updating rate limit profile: %w", err)
	}
	return nil
}

// DeleteRateLimitProfile removes a profile.
func (c *Client) DeleteRateLimitProfile(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", rateLimitPath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting rate limit profile: %w", err)
	}
	return nil
}
