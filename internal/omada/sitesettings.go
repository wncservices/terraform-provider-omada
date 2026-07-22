// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Site settings are one large singleton object (GET /setting) made of grouped
// sub-objects (led, mesh, roaming, beaconControl, ...). This provider manages
// the configurable groups; everything else in the object — notably
// `deviceAccount` (a credential) and read-only metadata — is left untouched by
// the read-modify-write in PatchSiteSettings. Update uses PATCH.

func settingPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting", siteID)
}

// GetSiteSettings returns the raw site-settings object.
func (c *Client) GetSiteSettings(ctx context.Context, siteID string) (map[string]any, error) {
	var out map[string]any
	if err := c.Do(ctx, "GET", settingPath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("reading site settings: %w", err)
	}
	return out, nil
}

// SettingBool reads a boolean from a settings group, e.g. ("mesh","meshEnable").
func SettingBool(st map[string]any, group, key string) (bool, bool) {
	g, ok := st[group].(map[string]any)
	if !ok {
		return false, false
	}
	v, ok := g[key].(bool)
	return v, ok
}

// SettingInt reads a number from a settings group (JSON numbers are float64).
func SettingInt(st map[string]any, group, key string) (int64, bool) {
	g, ok := st[group].(map[string]any)
	if !ok {
		return 0, false
	}
	f, ok := g[key].(float64)
	return int64(f), ok
}

// PatchSiteSettings applies grouped overrides. Each touched group is merged on
// top of its current value so unmanaged keys inside that group survive, and
// groups that aren't mentioned are not sent at all.
func (c *Client) PatchSiteSettings(ctx context.Context, siteID string, groups map[string]map[string]any) error {
	if len(groups) == 0 {
		return nil
	}
	cur, err := c.GetSiteSettings(ctx, siteID)
	if err != nil {
		return err
	}
	payload := map[string]any{}
	for g, kv := range groups {
		merged := map[string]any{}
		if base, ok := cur[g].(map[string]any); ok {
			for k, v := range base {
				merged[k] = v
			}
		}
		for k, v := range kv {
			merged[k] = v
		}
		payload[g] = merged
	}
	if err := c.Do(ctx, "PATCH", settingPath(siteID), payload, nil); err != nil {
		return fmt.Errorf("updating site settings: %w", err)
	}
	return nil
}
