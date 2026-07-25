// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDisableNATResource drives create → import → update of omada_disable_nat.
//
// The asymmetric paths are what this really guards: the collection is plural
// (`disable-nats`) while create and the item path are singular (`disable-nat`),
// and update is PUT — the mock answers -1600 to PATCH exactly as the controller
// does, so getting either wrong fails here rather than on real hardware.
func TestAccDisableNATResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_disable_nat" "guest" {
  name        = "test"
  interface   = "1_c967cf39292e474291e409b4dfe7f0cd"
  network_ids = ["net-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_disable_nat.guest", "id"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "name", "test"),
					// Defaults to disabled: enabling it strands the network
					// unless upstream routing exists.
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "enable", "false"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "network_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "network_ids.0", "net-1"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_disable_nat.guest",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_disable_nat" "guest" {
  name        = "test renamed"
  interface   = "1_c967cf39292e474291e409b4dfe7f0cd"
  network_ids = ["net-1", "net-2"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "name", "test renamed"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "network_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_disable_nat.guest", "enable", "false"),
				),
			},
		},
	})
}
