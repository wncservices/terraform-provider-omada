// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSiteSettingsResource covers the site-settings singleton across several
// groups (LED, mesh, roaming, band steering, RF beacon control), plus import by
// site name. Requires TF_ACC=1.
func TestAccSiteSettingsResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // unset attributes must read through from the controller
				Config: testProviderConfig(srv.URL) + `
resource "omada_site_settings" "this" {
  site       = "Default"
  led_enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_site_settings.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "led_enable", "false"),
					// read through, not managed in config:
					resource.TestCheckResourceAttr("omada_site_settings.this", "mesh_enable", "true"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "roaming_fast_roaming_enable", "true"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "band_steering_connection_threshold", "30"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "beacon_rts_threshold_2g", "2347"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "beacon_interval_6g", "100"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "speed_test_interval", "120"),
				),
			},
			{ResourceName: "omada_site_settings.this", ImportState: true, ImportStateVerify: true, ImportStateId: "Default"},
			{ // change values across several groups, including RF tuning
				Config: testProviderConfig(srv.URL) + `
resource "omada_site_settings" "this" {
  site                        = "Default"
  led_enable                  = true
  mesh_enable                 = false
  roaming_ai_roaming_enable   = true
  band_steering_max_failures  = 7
  beacon_dtim_period_2g       = 3
  remote_log_enable           = true
  remote_log_port             = 1514
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_site_settings.this", "led_enable", "true"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "mesh_enable", "false"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "roaming_ai_roaming_enable", "true"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "band_steering_max_failures", "7"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "beacon_dtim_period_2g", "3"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "remote_log_port", "1514"),
					// untouched keys inside a partially-updated group survive
					resource.TestCheckResourceAttr("omada_site_settings.this", "mesh_full_sector", "true"),
					resource.TestCheckResourceAttr("omada_site_settings.this", "remote_log_more_client_log", "false"),
				),
			},
		},
	})
}
