// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccNetworkResource drives create → import → update of omada_network
// against the in-test mock controller. Requires TF_ACC=1; no real hardware.
func TestAccNetworkResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testProviderConfig(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]
  dhcp_enabled   = true
  dhcp_start     = "10.10.40.100"
  dhcp_end       = "10.10.40.200"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_network.test", "id"),
					resource.TestCheckResourceAttr("omada_network.test", "name", "Lab"),
					resource.TestCheckResourceAttr("omada_network.test", "vlan_id", "40"),
					resource.TestCheckResourceAttr("omada_network.test", "purpose", "interface"),
					resource.TestCheckResourceAttr("omada_network.test", "gateway_subnet", "10.10.40.1/24"),
					resource.TestCheckResourceAttr("omada_network.test", "interface_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_enabled", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_start", "10.10.40.100"),
					resource.TestCheckResourceAttr("omada_network.test", "site_id", "site-1"),
				),
			},
			{ // import by network id
				ResourceName:      "omada_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // update: rename + disable dhcp
				Config: testProviderConfig(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab-renamed"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  dhcp_enabled   = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_network.test", "name", "Lab-renamed"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_enabled", "false"),
				),
			},
		},
	})
}
