// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Portal auth types.
const (
	PortalAuthNone           = 0 // click-through / no auth
	PortalAuthSimplePassword = 1 // one shared password
)

// AuthTimeout is the portal session-timeout sub-object.
type AuthTimeout struct {
	AuthTimeout       int `json:"authTimeout"`
	CustomTimeout     int `json:"customTimeout,omitempty"`
	CustomTimeoutUnit int `json:"customTimeoutUnit,omitempty"`
}

// Portal is a captive-portal configuration. Verified against a live v6.2
// controller (/setting/portals): create is POST, update is PATCH with a full
// read-modify-write payload, delete is DELETE.
//
// The landing-page look (portalCustomize) and other unmodelled sub-objects are
// preserved on update via deep-merge, so customising the page in the UI is not
// clobbered by Terraform.
//
// simplePassword.password is WRITE-ONLY: the controller never returns it, and
// this client never surfaces it into Terraform state.
type Portal struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Enable      bool        `json:"enable"`
	AuthType    int         `json:"authType"`
	SSIDList    []string    `json:"ssidList"`
	NetworkList []string    `json:"networkList"`
	AuthTimeout AuthTimeout `json:"authTimeout"`
	HTTPSRedir  bool        `json:"httpsRedirectEnable"`
	LandingPage int         `json:"landingPage"`
}

func portalsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/portals", siteID)
}

// ListPortals returns all portals for a site.
//
// This endpoint returns a bare JSON array in `result` (not the paginated
// {data,totalRows} envelope), so it is decoded directly rather than via the pager.
func (c *Client) ListPortals(ctx context.Context, siteID string) ([]Portal, error) {
	var portals []Portal
	if err := c.Do(ctx, "GET", portalsPath(siteID), nil, &portals); err != nil {
		return nil, fmt.Errorf("listing portals: %w", err)
	}
	return portals, nil
}

// GetPortal returns one portal by id.
func (c *Client) GetPortal(ctx context.Context, siteID, id string) (*Portal, error) {
	portals, err := c.ListPortals(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range portals {
		if portals[i].ID == id {
			return &portals[i], nil
		}
	}
	return nil, fmt.Errorf("portal %q not found on site %q", id, siteID)
}

// rawPortalByID fetches one portal as a raw map (for read-modify-write). The
// list endpoint is a bare array, so RawByID/RawList (which expect the paginated
// envelope) cannot be used here.
func (c *Client) rawPortalByID(ctx context.Context, siteID, id string) (map[string]any, error) {
	var raw []map[string]any
	if err := c.Do(ctx, "GET", portalsPath(siteID), nil, &raw); err != nil {
		return nil, fmt.Errorf("reading portals: %w", err)
	}
	for _, m := range raw {
		if s, _ := m["id"].(string); s == id {
			return m, nil
		}
	}
	return nil, fmt.Errorf("portal %q not found on site %q", id, siteID)
}

// portalDeepKeys are sub-objects that must be merged, never replaced — most
// importantly portalCustomize (the whole landing-page design) and simplePassword
// (so an update that doesn't set a password keeps the existing one).
var portalDeepKeys = []string{"portalCustomize", "authTimeout", "simplePassword", "noAuth", "advertisement"}

// withPassword places the shared portal password under simplePassword.password.
func withPassword(fields map[string]any, password string) {
	if password == "" {
		return
	}
	sp, _ := fields["simplePassword"].(map[string]any)
	if sp == nil {
		sp = map[string]any{}
	}
	sp["password"] = password
	fields["simplePassword"] = sp
}

// CreatePortal creates a portal (the controller returns a null result, so the
// new portal is resolved by name). password, if given, is written to
// simplePassword.password and never read back.
func (c *Client) CreatePortal(ctx context.Context, siteID string, fields map[string]any, password string) (*Portal, error) {
	withPassword(fields, password)
	name, _ := fields["name"].(string)
	if err := c.Do(ctx, "POST", portalsPath(siteID), fields, nil); err != nil {
		return nil, fmt.Errorf("creating portal: %w", err)
	}
	portals, err := c.ListPortals(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range portals {
		if portals[i].Name == name {
			return &portals[i], nil
		}
	}
	return nil, fmt.Errorf("portal %q not found after create", name)
}

// UpdatePortal does a read-modify-write. The controller rejects a partial PATCH
// (it validates the whole portalCustomize block), so the current object is
// fetched, the managed fields overlaid, and the result sent back. Nested
// sub-objects are deep-merged so the landing-page design and the existing
// password survive.
func (c *Client) UpdatePortal(ctx context.Context, siteID, id string, fields map[string]any, password string) (*Portal, error) {
	cur, err := c.rawPortalByID(ctx, siteID, id)
	if err != nil {
		return nil, err
	}
	mergeInto(cur, fields, portalDeepKeys...)
	withPassword(cur, password)
	// modifyTimeMs is controller-owned bookkeeping; sending it back is harmless
	// but pointless, and resource/siteId are read-only.
	delete(cur, "modifyTimeMs")
	if err := c.Do(ctx, "PATCH", portalsPath(siteID)+"/"+id, cur, nil); err != nil {
		return nil, fmt.Errorf("updating portal %q: %w", id, err)
	}
	return c.GetPortal(ctx, siteID, id)
}

// DeletePortal removes a portal.
func (c *Client) DeletePortal(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", portalsPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting portal %q: %w", id, err)
	}
	return nil
}
