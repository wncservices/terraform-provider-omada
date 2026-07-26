// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccNotificationSettingsResource drives create → import → update.
//
// The property this really guards is sparseness. The controller carries 131
// notifications; a configuration naming one of them must leave the other 130
// alone, and must not strip the controller-owned descriptive fields
// (shortMsg, module, level, deviceTypes) off the entry it does touch.
func TestAccNotificationSettingsResource(t *testing.T) {
	srv := newMockController(t)

	// entry returns one stored notification entry by key.
	entry := func(t *testing.T, list, key string) map[string]any {
		t.Helper()
		resp, err := http.Get(srv.URL + "/debug/notifications")
		if err != nil {
			t.Fatalf("debug fetch: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		var all map[string]map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
			t.Fatalf("decode: %v", err)
		}
		items, _ := all["logs"][list].([]any)
		for _, it := range items {
			e, _ := it.(map[string]any)
			if k, _ := e["key"].(string); k == key {
				return e
			}
		}
		t.Fatalf("entry %s/%s not found", list, key)
		return nil
	}

	// An untouched entry keeps its values; a touched one keeps its description.
	checkUntouched := func(*terraform.State) error {
		e := entry(t, "alertNotifications", "OSW_DET_LOOP")
		if v, _ := e["enable"].(bool); !v {
			return fmt.Errorf("undeclared entry OSW_DET_LOOP was modified: %v", e)
		}
		return nil
	}
	checkDescriptionPreserved := func(*terraform.State) error {
		e := entry(t, "alertNotifications", "OSW_DET_STORM")
		for _, k := range []string{"shortMsg", "module", "level", "deviceTypes"} {
			if _, ok := e[k]; !ok {
				return fmt.Errorf("controller-owned field %q was dropped: %v", k, e)
			}
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_notification_settings" "this" {
  alert_email_enable = true
  webhook_enable     = true

  alert = {
    OSW_DET_STORM = {
      email  = false
      enable = false
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert_email_enable", "true"),
					resource.TestCheckResourceAttr("omada_notification_settings.this", "webhook_enable", "true"),
					// Never set: comes back from the controller.
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert_email_delay", "60"),
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert.OSW_DET_STORM.enable", "false"),
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert.OSW_DET_STORM.email", "false"),
					// Only the declared key appears — not all 131.
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert.%", "1"),
					checkUntouched,
					checkDescriptionPreserved,
				),
			},
			{
				ResourceName:      "omada_notification_settings.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
				// The sparse maps are configuration, not something import can
				// discover: there is no way to know which of 131 notifications
				// the practitioner intended to manage.
				ImportStateVerifyIgnore: []string{"alert", "event"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_notification_settings" "this" {
  alert_email_enable = false
  webhook_enable     = true

  alert = {
    OSW_DET_STORM = {
      email  = true
      enable = true
    }
  }

  event = {
    DEV_IP_C = {
      enable = true
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert_email_enable", "false"),
					resource.TestCheckResourceAttr("omada_notification_settings.this", "alert.OSW_DET_STORM.enable", "true"),
					resource.TestCheckResourceAttr("omada_notification_settings.this", "event.DEV_IP_C.enable", "true"),
					checkUntouched,
					checkDescriptionPreserved,
				),
			},
		},
	})
}
