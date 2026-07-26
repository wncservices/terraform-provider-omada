// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Notification settings live outside /setting/, under two sibling documents:
//
//	GET/PATCH /logs/notification         alert + event notifications, email, webhook
//	GET/PATCH /site/audit-notification   audit-log notifications
//
// Both take `PATCH` with the **whole document** (`PUT` and `POST` are -1600,
// and an empty PATCH body is rejected -1001), so updates are read-modify-write.
//
// The interesting part is scale: /logs/notification carries 63 alert and 68
// event entries, each a small object keyed by a stable `key` like
// `OSW_DET_STORM`. Most of each entry is controller-owned description —
// `shortMsg`, `module`, `level`, `deviceTypes` — and only `email`, `webhook`
// and `enable` are configuration. So the update below patches entries **in
// place by key** and leaves every other entry, and every unmodelled field,
// exactly as the controller had it. That lets a practitioner name the handful
// of notifications they care about instead of restating 131 of them.

// NotificationToggle is the configurable part of one notification entry.
type NotificationToggle struct {
	Email   *bool
	Webhook *bool
	Enable  *bool
}

// notificationDoc identifies one of the two notification documents.
type notificationDoc struct {
	path string
	// lists names the keyed entry lists inside the document.
	lists []string
}

var (
	// NotificationSettingsDoc is the alert/event notification document.
	NotificationSettingsDoc = notificationDoc{
		path:  "/logs/notification",
		lists: []string{"alertNotifications", "eventNotifications"},
	}
	// AuditNotificationDoc is the audit-log notification document.
	AuditNotificationDoc = notificationDoc{
		path:  "/site/audit-notification",
		lists: []string{"logNotifications"},
	}
)

func (d notificationDoc) fullPath(siteID string) string {
	return fmt.Sprintf("/sites/%s%s", siteID, d.path)
}

// GetNotificationDoc reads one notification document.
func (c *Client) GetNotificationDoc(ctx context.Context, siteID string, d notificationDoc) (map[string]any, error) {
	var out map[string]any
	if err := c.Do(ctx, http.MethodGet, d.fullPath(siteID), nil, &out); err != nil {
		return nil, fmt.Errorf("reading %s: %w", d.path, err)
	}
	return out, nil
}

// NotificationEntries returns the keyed entries of one list, by key.
func NotificationEntries(doc map[string]any, list string) map[string]map[string]any {
	out := map[string]map[string]any{}
	items, _ := doc[list].([]any)
	for _, it := range items {
		e, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if k, _ := e["key"].(string); k != "" {
			out[k] = e
		}
	}
	return out
}

// UpdateNotificationDoc read-modify-writes a notification document: `groups`
// merges into the top-level sub-objects (alertEmailSetting, webhookSetting, …),
// and `toggles` patches individual entries by key within the named list.
//
// Entries and fields not mentioned are left exactly as the controller had
// them, which is what keeps a configuration from having to restate every
// notification the controller knows about.
func (c *Client) UpdateNotificationDoc(
	ctx context.Context,
	siteID string,
	d notificationDoc,
	groups map[string]map[string]any,
	toggles map[string]map[string]NotificationToggle,
) error {
	cur, err := c.GetNotificationDoc(ctx, siteID, d)
	if err != nil {
		return err
	}
	delete(cur, "resource")

	for group, kv := range groups {
		base, _ := cur[group].(map[string]any)
		merged := map[string]any{}
		for k, v := range base {
			merged[k] = v
		}
		for k, v := range kv {
			merged[k] = v
		}
		cur[group] = merged
	}

	for list, byKey := range toggles {
		entries := NotificationEntries(cur, list)
		for key, t := range byKey {
			e, ok := entries[key]
			if !ok {
				return fmt.Errorf("%s has no notification with key %q", d.path, key)
			}
			if t.Email != nil {
				e["email"] = *t.Email
			}
			if t.Webhook != nil {
				e["webhook"] = *t.Webhook
			}
			if t.Enable != nil {
				e["enable"] = *t.Enable
			}
		}
	}

	if err := c.Do(ctx, http.MethodPatch, d.fullPath(siteID), cur, nil); err != nil {
		return fmt.Errorf("updating %s: %w", d.path, err)
	}
	return nil
}
