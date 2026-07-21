// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccFirewallACLResource drives create → import → update of
// omada_firewall_acl against the in-test mock. Requires TF_ACC=1.
func TestAccFirewallACLResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create — permit mgmt network to two others
				Config: testProviderConfig(srv.URL) + `
resource "omada_firewall_acl" "mgmt" {
  name            = "MGMT allow"
  policy          = "permit"
  protocols       = [256]
  source_ids      = ["net-mgmt"]
  destination_ids = ["net-a", "net-b"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_firewall_acl.mgmt", "id"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "policy", "permit"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "type", "gateway"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "enable", "true"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "source_ids.0", "net-mgmt"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "destination_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "direction.lan_to_lan", "true"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "site_id", "site-1"),
				),
			},
			{ // import
				ResourceName:      "omada_firewall_acl.mgmt",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // update: deny + single destination
				Config: testProviderConfig(srv.URL) + `
resource "omada_firewall_acl" "mgmt" {
  name            = "MGMT deny"
  policy          = "deny"
  protocols       = [6, 17]
  source_ids      = ["net-mgmt"]
  destination_ids = ["net-a"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "policy", "deny"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "protocols.#", "2"),
					resource.TestCheckResourceAttr("omada_firewall_acl.mgmt", "destination_ids.#", "1"),
				),
			},
		},
	})
}
