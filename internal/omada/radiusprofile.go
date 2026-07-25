// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RADIUS profiles (Settings -> Authentication -> RADIUS Profile) describe the
// servers that 802.1X, MAC authentication and WPA-Enterprise SSIDs authenticate
// against. `omada_dot1x` is not usable without one.
//
// Two things shape this file:
//
//   - Like /setting/portals and /devices, the list is a **bare JSON array**,
//     not a paginated envelope, so it is decoded directly rather than via the
//     pager.
//   - The shared secret lives at `authServer[].radiusPwd` and the controller
//     **returns it in plaintext** on read, exactly like the WiFi psk. Per
//     DESIGN.md §2.6 it is therefore write-only here: never decoded into a
//     typed struct, never handed back to the provider, and carried across an
//     update that does not set a new one.

const radiusPwdKey = "radiusPwd"

func radiusProfilePath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/radiusProfiles", siteID)
}

// rawRadiusProfiles fetches the profile list as raw maps.
func (c *Client) rawRadiusProfiles(ctx context.Context, siteID string) ([]map[string]any, error) {
	var out []map[string]any
	if err := c.Do(ctx, http.MethodGet, radiusProfilePath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("listing radius profiles: %w", err)
	}
	return out, nil
}

// RadiusProfileSummary is the read-side view. It deliberately has no field for
// the shared secret.
type RadiusProfileSummary struct {
	ID           string             `json:"radiusProfileId"`
	Name         string             `json:"name"`
	AuthServers  []RadiusAuthServer `json:"authServer"`
	Accounting   bool               `json:"radiusAccountingEnable"`
	InterimUpd   bool               `json:"interimUpdateEnable"`
	VlanAssign   bool               `json:"wirelessVlanAssignment"`
	DomainEnable bool               `json:"domainEnable"`
	CoaEnable    bool               `json:"coaEnable"`
	Proxy        bool               `json:"proxy"`
	RequireMsgAu bool               `json:"requireMessageAuthenticator"`
	BuiltIn      bool               `json:"builtInServer"`
	ServerEnable bool               `json:"serverEnable"`
	TunnelReply  bool               `json:"tunnelReplyEnable"`
}

// RadiusAuthServer is one authentication server. Note the absence of the
// secret: it is never read back into memory.
type RadiusAuthServer struct {
	IP          string `json:"radiusServerIp"`
	Port        int    `json:"radiusPort"`
	RadSecEnabl bool   `json:"radSecEnable"`
}

// ListRadiusProfiles returns every profile, without secrets.
func (c *Client) ListRadiusProfiles(ctx context.Context, siteID string) ([]RadiusProfileSummary, error) {
	raws, err := c.rawRadiusProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	out := make([]RadiusProfileSummary, 0, len(raws))
	for _, m := range raws {
		stripRadiusSecrets(m)
		buf, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("re-encoding radius profile: %w", err)
		}
		var p RadiusProfileSummary
		if err := json.Unmarshal(buf, &p); err != nil {
			return nil, fmt.Errorf("decoding radius profile: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// GetRadiusProfile returns one profile by id, without secrets.
func (c *Client) GetRadiusProfile(ctx context.Context, siteID, id string) (*RadiusProfileSummary, error) {
	items, err := c.ListRadiusProfiles(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("radius profile %q not found", id)
}

// stripRadiusSecrets removes plaintext shared secrets from a decoded profile so
// they cannot leak into logs, state or diagnostics.
func stripRadiusSecrets(m map[string]any) {
	for _, key := range []string{"authServer", "acctServer"} {
		list, ok := m[key].([]any)
		if !ok {
			continue
		}
		for _, entry := range list {
			if e, ok := entry.(map[string]any); ok {
				delete(e, radiusPwdKey)
			}
		}
	}
}

// carryRadiusSecrets copies the existing shared secret into any server entry
// whose secret was left empty, so an update that does not set a new secret does
// not blank the one already configured. Entries are matched by position, which
// is how the controller returns and stores them.
func carryRadiusSecrets(next, cur []any) {
	for i, entry := range next {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if pw, _ := e[radiusPwdKey].(string); pw != "" {
			continue
		}
		delete(e, radiusPwdKey)
		if i >= len(cur) {
			continue
		}
		old, ok := cur[i].(map[string]any)
		if !ok {
			continue
		}
		if pw, _ := old[radiusPwdKey].(string); pw != "" {
			e[radiusPwdKey] = pw
		}
	}
}

// CreateRadiusProfile creates a profile and returns its id.
func (c *Client) CreateRadiusProfile(ctx context.Context, siteID string, fields map[string]any) (string, error) {
	var out struct {
		ID string `json:"radiusProfileId"`
	}
	if err := c.Do(ctx, http.MethodPost, radiusProfilePath(siteID), fields, &out); err != nil {
		return "", fmt.Errorf("creating radius profile: %w", err)
	}
	if out.ID != "" {
		return out.ID, nil
	}
	name, _ := fields["name"].(string)
	items, err := c.ListRadiusProfiles(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created radius profile %q but could not resolve its id", name)
}

// UpdateRadiusProfile read-modify-writes a profile: fields the provider does
// not model survive, and a shared secret that was not re-supplied is preserved.
func (c *Client) UpdateRadiusProfile(ctx context.Context, siteID, id string, fields map[string]any) error {
	raws, err := c.rawRadiusProfiles(ctx, siteID)
	if err != nil {
		return err
	}
	var cur map[string]any
	for _, m := range raws {
		if s, _ := m["radiusProfileId"].(string); s == id {
			cur = m
			break
		}
	}
	if cur == nil {
		return fmt.Errorf("radius profile %q not found", id)
	}

	if next, ok := fields["authServer"].([]any); ok {
		existing, _ := cur["authServer"].([]any)
		carryRadiusSecrets(next, existing)
	}

	mergeInto(cur, fields)
	delete(cur, "resource")

	path := fmt.Sprintf("%s/%s", radiusProfilePath(siteID), id)
	if err := c.Do(ctx, http.MethodPatch, path, cur, nil); err != nil {
		return fmt.Errorf("updating radius profile: %w", err)
	}
	return nil
}

// DeleteRadiusProfile removes a profile.
func (c *Client) DeleteRadiusProfile(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", radiusProfilePath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting radius profile: %w", err)
	}
	return nil
}
