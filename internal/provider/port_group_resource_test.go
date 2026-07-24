// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPortGroupResource drives create → import → update of omada_port_group
// against the in-test mock controller. Requires TF_ACC=1.
func TestAccPortGroupResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_group" "test" {
  name  = "web-ports"
  ports = ["8080", "9000-9100"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_port_group.test", "id"),
					resource.TestCheckResourceAttr("omada_port_group.test", "port_type", "0"),
					resource.TestCheckResourceAttr("omada_port_group.test", "ports.#", "2"),
					resource.TestCheckResourceAttr("omada_port_group.test", "ports.0", "8080"),
					resource.TestCheckResourceAttr("omada_port_group.test", "ports.1", "9000-9100"),
					resource.TestCheckResourceAttr("omada_port_group.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_port_group.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_group" "test" {
  name  = "web-ports"
  ports = ["443"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_group.test", "ports.#", "1"),
					resource.TestCheckResourceAttr("omada_port_group.test", "ports.0", "443"),
				),
			},
		},
	})
}
