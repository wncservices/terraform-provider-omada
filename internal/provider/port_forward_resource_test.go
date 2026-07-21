// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPortForwardResource drives create → import → update of
// omada_port_forward against the in-test mock. Requires TF_ACC=1.
func TestAccPortForwardResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create — wan_port_ids omitted, inherited from the seeded rule
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_forward" "https" {
  name          = "Homelab"
  external_port = "443"
  forward_ip    = "10.10.20.95"
  forward_port  = "443"
  protocol      = "tcp"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_port_forward.https", "id"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "external_port", "443"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "protocol", "tcp"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "enable", "true"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "wan_port_ids.0", "wan-1"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "site_id", "site-1"),
				),
			},
			{ // import
				ResourceName:      "omada_port_forward.https",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // update: switch to udp + change forward port
				Config: testProviderConfig(srv.URL) + `
resource "omada_port_forward" "https" {
  name          = "Homelab"
  external_port = "443"
  forward_ip    = "10.10.20.95"
  forward_port  = "8443"
  protocol      = "udp"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_forward.https", "forward_port", "8443"),
					resource.TestCheckResourceAttr("omada_port_forward.https", "protocol", "udp"),
				),
			},
		},
	})
}
