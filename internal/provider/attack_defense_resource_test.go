// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAttackDefenseResource drives create → import → update of the
// attack-defense singleton, and asserts the property that matters for every
// read-modify-write resource: controller keys the provider does not model are
// still there afterwards.
func TestAccAttackDefenseResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["firewall/attackdefense"]
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
resource "omada_attack_defense" "this" {
  ping_wan_enable  = false
  tcp_conn_enable  = true
  ip_option_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_attack_defense.this", "ping_wan_enable", "false"),
					resource.TestCheckResourceAttr("omada_attack_defense.this", "tcp_conn_enable", "true"),
					// Never set in config, so it comes back from the controller.
					resource.TestCheckResourceAttr("omada_attack_defense.this", "tcp_winnuke_enable", "true"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_attack_defense.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_attack_defense" "this" {
  ping_wan_enable  = true
  tcp_conn_enable  = false
  ip_option_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_attack_defense.this", "ping_wan_enable", "true"),
					resource.TestCheckResourceAttr("omada_attack_defense.this", "tcp_conn_enable", "false"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
