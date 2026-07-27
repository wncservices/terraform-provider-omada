// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccGRETunnelResource drives create → import → update of the GRE tunnel
// setting.
func TestAccGRETunnelResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["vpns/greTunnel"]
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
resource "omada_gre_tunnel" "this" {
  enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "enable", "false"),
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "ssid_ids.#", "0"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_gre_tunnel.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_gre_tunnel" "this" {
  enable   = true
  ssid_ids = ["ssid-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "ssid_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_gre_tunnel.this", "ssid_ids.0", "ssid-1"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
