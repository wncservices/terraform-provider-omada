// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIPGroupResource drives create → import → update of omada_ip_group
// against the in-test mock. Requires TF_ACC=1.
func TestAccIPGroupResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testProviderConfig(srv.URL) + `
resource "omada_ip_group" "test" {
  name = "lab-nets"
  ip_list = [
    { ip = "10.10.10.0", mask = 24 },
    { ip = "10.10.20.0", mask = 24, description = "service" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_ip_group.test", "id"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "name", "lab-nets"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.#", "2"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.0.ip", "10.10.10.0"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.1.description", "service"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "site_id", "site-1"),
				),
			},
			{ // import
				ResourceName:      "omada_ip_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // update: single entry
				Config: testProviderConfig(srv.URL) + `
resource "omada_ip_group" "test" {
  name = "lab-nets"
  ip_list = [
    { ip = "10.10.99.0", mask = 24, description = "mgmt" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.#", "1"),
					resource.TestCheckResourceAttr("omada_ip_group.test", "ip_list.0.ip", "10.10.99.0"),
				),
			},
		},
	})
}
