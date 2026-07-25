// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIPSWhitelistResource drives create → import → replace.
//
// There is no update step because there is nothing to update: an entry is
// only direction + traffic_type + traffic_source, so changing any of them is
// a different rule. The third step asserts that a changed traffic_source
// replaces rather than attempting an in-place update the controller has no
// verb for.
func TestAccIPSWhitelistResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_ips_whitelist" "guest" {
  direction      = 1
  traffic_type   = 1
  traffic_source = "net-1"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_ips_whitelist.guest", "id"),
					resource.TestCheckResourceAttr("omada_ips_whitelist.guest", "direction", "1"),
					resource.TestCheckResourceAttr("omada_ips_whitelist.guest", "traffic_type", "1"),
					resource.TestCheckResourceAttr("omada_ips_whitelist.guest", "traffic_source", "net-1"),
					resource.TestCheckResourceAttr("omada_ips_whitelist.guest", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_ips_whitelist.guest",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_ips_whitelist" "guest" {
  direction      = 1
  traffic_type   = 1
  traffic_source = "net-2"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ips_whitelist.guest", "traffic_source", "net-2"),
				),
			},
		},
	})
}
