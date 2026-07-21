// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccLanDNSResource drives create → import → update of omada_lan_dns against
// the in-test mock controller. Requires TF_ACC=1; no real hardware.
func TestAccLanDNSResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testProviderConfig(srv.URL) + `
resource "omada_lan_dns" "test" {
  name            = "nas"
  domain          = "nas.wilant.be"
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = ["net-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_lan_dns.test", "id"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "domain", "nas.wilant.be"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "enable", "true"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "ip_addresses.#", "1"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "lan_network_ids.0", "net-1"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "site_id", "site-1"),
				),
			},
			{ // import
				ResourceName:      "omada_lan_dns.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{ // update: change domain + add an alias
				Config: testProviderConfig(srv.URL) + `
resource "omada_lan_dns" "test" {
  name            = "nas"
  domain          = "storage.wilant.be"
  aliases         = ["files.wilant.be"]
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = ["net-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_dns.test", "domain", "storage.wilant.be"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "aliases.0", "files.wilant.be"),
				),
			},
		},
	})
}
