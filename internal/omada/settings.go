// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Several controller settings are *flat singleton documents*: one JSON object
// per site behind a fixed path, with no id, that cannot be created or deleted —
// only read and updated. SSH, ALG and attack defense are all of this shape.
//
// Two things make them awkward enough to justify a shared helper:
//
//  1. The update verb is not consistent. `/setting/ssh`,
//     `/setting/transmission/alg` and `/setting/firewall/attackdefense` reject
//     PATCH with -1600 and require PUT; `/setting/dot1x` and
//     `/setting/accessControl` are the other way round. Each SettingDoc below
//     records the verb that was confirmed against a live controller.
//  2. Reads carry controller-owned metadata that must not be echoed back — the
//     `support*` / `exist*` capability flags the UI uses to decide what to
//     render, and a `resource` counter. Sending them back is at best noise and
//     at worst rejected, so updateSetting strips them.

// SettingDoc locates a flat singleton settings document and records the verb
// that updates it.
type SettingDoc struct {
	// Path is appended to /sites/{siteID}.
	Path string
	// Verb is the confirmed update method (http.MethodPut or http.MethodPatch).
	Verb string
}

// The singleton settings documents this provider manages. Every verb here was
// confirmed against a live v6.2 controller by writing the document's own
// current values back and checking the result was accepted and unchanged.
var (
	SSHSetting           = SettingDoc{Path: "/setting/ssh", Verb: http.MethodPut}
	ALGSetting           = SettingDoc{Path: "/setting/transmission/alg", Verb: http.MethodPut}
	AttackDefenseSetting = SettingDoc{Path: "/setting/firewall/attackdefense", Verb: http.MethodPut}
)

func (d SettingDoc) path(siteID string) string {
	return fmt.Sprintf("/sites/%s%s", siteID, d.Path)
}

// controllerOwnedKey reports whether k is metadata the controller adds to reads
// and that must not be sent back on update.
func controllerOwnedKey(k string) bool {
	if k == "resource" {
		return true
	}
	for _, p := range []string{"support", "exist", "osgSupport"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// GetSetting reads a flat singleton settings document.
func (c *Client) GetSetting(ctx context.Context, siteID string, doc SettingDoc) (map[string]any, error) {
	var out map[string]any
	if err := c.Do(ctx, http.MethodGet, doc.path(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("reading %s: %w", doc.Path, err)
	}
	return out, nil
}

// UpdateSetting read-modify-writes a flat singleton settings document: it
// merges fields into the document's current value so keys this provider does
// not model survive untouched, strips controller-owned metadata, and sends the
// whole document back with the verb confirmed for that endpoint.
func (c *Client) UpdateSetting(ctx context.Context, siteID string, doc SettingDoc, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	cur, err := c.GetSetting(ctx, siteID, doc)
	if err != nil {
		return err
	}
	for k := range cur {
		if controllerOwnedKey(k) {
			delete(cur, k)
		}
	}
	mergeInto(cur, fields)
	if err := c.Do(ctx, doc.Verb, doc.path(siteID), cur, nil); err != nil {
		return fmt.Errorf("updating %s: %w", doc.Path, err)
	}
	return nil
}
