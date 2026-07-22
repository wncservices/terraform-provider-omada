// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSiteSettingsResource drives create → import → update of
// omada_site_settings (singleton) against the in-test mock. Requires TF_ACC=1.
func TestAccSiteSettingsResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_site_settings" "this" {
  site       = "Default"
  led_enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_site_settings.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "led_enable", "false"),
				),
			},
			{ResourceName: "omada_site_settings.this", ImportState: true, ImportStateVerify: true, ImportStateId: "Default"},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_site_settings" "this" {
  site       = "Default"
  led_enable = true
}`,
				Check: resource.TestCheckResourceAttr("omada_site_settings.this", "led_enable", "true"),
			},
		},
	})
}
