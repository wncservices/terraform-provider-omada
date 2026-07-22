// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Site settings are a large singleton object (GET /setting). This provider
// manages a small subset (currently the device LED toggle) via read-modify-write
// so the rest of the object is preserved. Update uses PATCH (the documented
// verb for this endpoint).

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

// GetLEDEnable reports whether the device status LEDs are on.
func (c *Client) GetLEDEnable(ctx context.Context, siteID string) (bool, error) {
	cur, err := c.GetSiteSettings(ctx, siteID)
	if err != nil {
		return false, err
	}
	if led, ok := cur["led"].(map[string]any); ok {
		if v, ok := led["enable"].(bool); ok {
			return v, nil
		}
	}
	return false, nil
}

// SetLEDEnable toggles the device status LEDs, preserving the rest of the led
// object (read-modify-write).
func (c *Client) SetLEDEnable(ctx context.Context, siteID string, enable bool) error {
	cur, err := c.GetSiteSettings(ctx, siteID)
	if err != nil {
		return err
	}
	led, _ := cur["led"].(map[string]any)
	if led == nil {
		led = map[string]any{}
	}
	led["enable"] = enable
	if err := c.Do(ctx, "PATCH", settingPath(siteID), map[string]any{"led": led}, nil); err != nil {
		return fmt.Errorf("updating site settings: %w", err)
	}
	return nil
}
