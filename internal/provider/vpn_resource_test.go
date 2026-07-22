// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccVPNResource drives create → import → update of omada_vpn against the
// in-test mock. Requires TF_ACC=1. (Real-controller write verbs are inferred.)
func TestAccVPNResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_vpn" "home" {
  name = "Home"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_vpn.home", "id"),
					resource.TestCheckResourceAttr("omada_vpn.home", "name", "Home"),
					resource.TestCheckResourceAttr("omada_vpn.home", "enable", "true"),
					resource.TestCheckResourceAttr("omada_vpn.home", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_vpn.home", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_vpn" "home" {
  name   = "Home"
  enable = false
}`,
				Check: resource.TestCheckResourceAttr("omada_vpn.home", "enable", "false"),
			},
		},
	})
}
