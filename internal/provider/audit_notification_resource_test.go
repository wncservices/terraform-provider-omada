// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAuditNotificationResource drives create → import → update. Audit
// categories carry only a `webhook` toggle, so this also exercises the
// toggle map without an `enable` attribute.
func TestAccAuditNotificationResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_audit_notification" "this" {
  webhook_enable = true

  log = {
    AUTHENTICATION = { webhook = true }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_audit_notification.this", "webhook_enable", "true"),
					resource.TestCheckResourceAttr("omada_audit_notification.this", "log.AUTHENTICATION.webhook", "true"),
					// Only the declared category is tracked.
					resource.TestCheckResourceAttr("omada_audit_notification.this", "log.%", "1"),
				),
			},
			{
				ResourceName:            "omada_audit_notification.this",
				ImportState:             true,
				ImportStateId:           "Default",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"log"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_audit_notification" "this" {
  webhook_enable = false

  log = {
    AUTHENTICATION = { webhook = false }
    CLIENTS        = { webhook = true }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_audit_notification.this", "webhook_enable", "false"),
					resource.TestCheckResourceAttr("omada_audit_notification.this", "log.CLIENTS.webhook", "true"),
					resource.TestCheckResourceAttr("omada_audit_notification.this", "log.%", "2"),
				),
			},
		},
	})
}
