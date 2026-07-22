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
//
// The update step deliberately omits `psk`: the WiFi key must survive, because
// the client deep-merges pskSetting instead of replacing it. That is asserted
// against the mock's raw stored object, since psk is never read into state.
func TestAccWirelessNetworkResource(t *testing.T) {
	srv := newMockController(t)

	checkKeyPreserved := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			for _, s := range rawStore(t, srv.URL, "ssids") {
				ps, _ := s["pskSetting"].(map[string]any)
				if ps == nil {
					return fmt.Errorf("ssid has no pskSetting: %v", s)
				}
				if got, _ := ps["securityKey"].(string); got != want {
					return fmt.Errorf("securityKey = %q, want %q (the WiFi key was clobbered)", got, want)
				}
			}
			return nil
		}
	}

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
  enable_11r    = true
  pmf_mode      = 2
  mac_filter_enable = false
  multicast_enable  = true
  multicast_channel_util = 100
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_wireless_network.iot", "id"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "vlan_id", "30"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "enable_11r", "true"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "pmf_mode", "2"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "multicast_channel_util", "100"),
					checkKeyPreserved("supersecret"),
				),
			},
			{
				ResourceName:            "omada_wireless_network.iot",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"psk"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources["omada_wireless_network.iot"]
					return fmt.Sprintf("%s/%s", rs.Primary.Attributes["wlan_group_id"], rs.Primary.Attributes["id"]), nil
				},
			},
			{ // update WITHOUT psk — the existing WiFi key must be preserved
				Config: testProviderConfig(srv.URL) + `
resource "omada_wireless_network" "iot" {
  wlan_group_id = "grp-default"
  name          = "IoT"
  vlan_enable   = true
  vlan_id       = 40
  enable_11r    = false
  mac_filter_enable = true
  multicast_enable  = true
  multicast_channel_util = 80
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "vlan_id", "40"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "enable_11r", "false"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "mac_filter_enable", "true"),
					resource.TestCheckResourceAttr("omada_wireless_network.iot", "multicast_channel_util", "80"),
					checkKeyPreserved("supersecret"),
				),
			},
		},
	})
}
