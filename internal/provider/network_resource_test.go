// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccNetworkResource drives create → import → update of omada_network
// against the in-test mock, covering the per-VLAN toggles and DHCP options.
// Requires TF_ACC=1.
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
  isolation      = true
  dhcp_enabled   = true
  dhcp_start     = "10.10.40.100"
  dhcp_end       = "10.10.40.200"
  dhcp_lease_time = 240
  dhcp_dns_mode   = "auto"

  dhcp_options = [
    { code = 138, type = 1, value = "10.10.20.50" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_network.test", "id"),
					resource.TestCheckResourceAttr("omada_network.test", "vlan_id", "40"),
					resource.TestCheckResourceAttr("omada_network.test", "purpose", "interface"),
					resource.TestCheckResourceAttr("omada_network.test", "isolation", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_lease_time", "240"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_dns_mode", "auto"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.#", "1"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.0.code", "138"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.0.value", "10.10.20.50"),
					resource.TestCheckResourceAttr("omada_network.test", "interface_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_network.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_network.test", ImportState: true, ImportStateVerify: true},
			{ // update: flip isolation, change lease + a DHCP option
				Config: testProviderConfig(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]
  isolation      = false
  arp_detection_enable = true
  dhcp_enabled   = true
  dhcp_start     = "10.10.40.100"
  dhcp_end       = "10.10.40.200"
  dhcp_lease_time = 120
  dhcp_dns_mode   = "auto"

  dhcp_options = [
    { code = 138, type = 1, value = "10.10.99.5" },
    { code = 66,  type = 1, value = "10.10.20.60" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_network.test", "isolation", "false"),
					resource.TestCheckResourceAttr("omada_network.test", "arp_detection_enable", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_lease_time", "120"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.#", "2"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.1.code", "66"),
				),
			},
		},
	})
}
