// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPortProfileResource drives create → import → update of
// omada_port_profile against the in-test mock. Requires TF_ACC=1.
func TestAccPortProfileResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_profile" "test" {
  name               = "trunk"
  poe                = 2
  native_network_id  = "net-1"
  tagged_network_ids = ["net-a", "net-b"]
  vlan_config_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_port_profile.test", "id"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "name", "trunk"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "poe", "2"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "native_network_id", "net-1"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "tagged_network_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_port_profile.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_profile" "test" {
  name               = "trunk"
  poe                = 0
  native_network_id  = "net-1"
  tagged_network_ids = ["net-a"]
  vlan_config_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_profile.test", "poe", "0"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "tagged_network_ids.#", "1"),
				),
			},
		},
	})
}
