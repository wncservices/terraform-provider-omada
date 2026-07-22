// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccWirelessNetworkResource drives create → import → update of
// omada_wireless_network against the in-test mock. Requires TF_ACC=1.
func TestAccWirelessNetworkResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_wireless_network" "iot" {
  wlan_group_id = "grp-default"
  name          = "IoT"
  psk           = "supersecret"
  vlan_enable   = true
  vlan_id       = 30
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_wireless_network.iot", "id"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "name", "IoT"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "vlan_enable", "true"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "vlan_id", "30"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "broadcast", "true"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_wireless_network.iot",
				ImportState:       true,
				ImportStateVerify: true,
				// psk is write-only (not returned by the controller).
				ImportStateVerifyIgnore: []string{"psk"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["omada_wireless_network.iot"]
					return fmt.Sprintf("%s/%s", rs.Primary.Attributes["wlan_group_id"], rs.Primary.Attributes["id"]), nil
				},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_wireless_network" "iot" {
  wlan_group_id = "grp-default"
  name          = "IoT-2"
  psk           = "supersecret"
  vlan_enable   = true
  vlan_id       = 40
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "name", "IoT-2"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "vlan_id", "40"),
				),
			},
		},
	})
}
