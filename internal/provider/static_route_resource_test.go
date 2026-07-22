// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccStaticRouteResource drives create → import → update of
// omada_static_route. The mock rejects PATCH on the item path, matching the
// live controller, so this also pins the update verb to PUT. Requires TF_ACC=1.
func TestAccStaticRouteResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_static_route" "test" {
  name         = "to-lab"
  destinations = ["192.168.50.0/24"]
  next_hop_ip  = "10.10.20.1"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_static_route.test", "id"),
					resource.TestCheckResourceAttr("omada_static_route.test", "enable", "true"),
					resource.TestCheckResourceAttr("omada_static_route.test", "route_type", "0"),
					resource.TestCheckResourceAttr("omada_static_route.test", "metric", "0"),
					resource.TestCheckResourceAttr("omada_static_route.test", "destinations.#", "1"),
					resource.TestCheckResourceAttr("omada_static_route.test", "destinations.0", "192.168.50.0/24"),
				),
			},
			{ResourceName: "omada_static_route.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_static_route" "test" {
  name         = "to-lab"
  enable       = false
  destinations = ["192.168.50.0/24", "192.168.51.0/24"]
  next_hop_ip  = "10.10.20.2"
  metric       = 10
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_static_route.test", "enable", "false"),
					resource.TestCheckResourceAttr("omada_static_route.test", "next_hop_ip", "10.10.20.2"),
					resource.TestCheckResourceAttr("omada_static_route.test", "metric", "10"),
					resource.TestCheckResourceAttr("omada_static_route.test", "destinations.#", "2"),
				),
			},
		},
	})
}
