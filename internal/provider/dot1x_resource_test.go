// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccDot1XResource drives create → import → update of the 802.1X singleton.
//
// This one is the reason the singleton scaffold takes the verb from its
// SettingDoc rather than hard-coding it: dot1x updates with PATCH, while the
// ALG/attack-defense/SSH documents require PUT. The mock answers -1600 to the
// wrong verb, so this test fails if that wiring regresses.
func TestAccDot1XResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["dot1x"]
		if got, _ := doc["unmodelledKey"].(string); got != "keep-me" {
			return fmt.Errorf("unmodelled key was dropped: %v", doc)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dot1x" "this" {
  enable      = false
  vlan_assign = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dot1x.this", "enable", "false"),
					resource.TestCheckResourceAttr("omada_dot1x.this", "vlan_assign", "true"),
					// Never set in config, so it comes back from the controller.
					resource.TestCheckResourceAttr("omada_dot1x.this", "auth_mode", "1"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_dot1x.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dot1x" "this" {
  enable      = false
  vlan_assign = false
  mac_format  = 1
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dot1x.this", "vlan_assign", "false"),
					resource.TestCheckResourceAttr("omada_dot1x.this", "mac_format", "1"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
