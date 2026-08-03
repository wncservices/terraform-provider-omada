// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strings"
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
  domain          = "nas.example.internal"
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = ["net-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_lan_dns.test", "id"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "domain", "nas.example.internal"),
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
  domain          = "storage.example.internal"
  aliases         = ["files.example.internal"]
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = ["net-1"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_lan_dns.test", "domain", "storage.example.internal"),
					resource.TestCheckResourceAttr("omada_lan_dns.test", "aliases.0", "files.example.internal"),
				),
			},
		},
	})
}

// TestAccLanDNSResourceAliasLimit pins the alias ceiling to plan time. The
// controller rejects an over-long list with "Size of aliases should be less
// than 7", but only at apply — by which point the rest of the run has already
// been applied. maxLanDNSAliases entries must still pass.
func TestAccLanDNSResourceAliasLimit(t *testing.T) {
	srv := newMockController(t)

	aliasList := func(n int) string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%q", fmt.Sprintf("alias-%d.example.internal", i))
		}
		return strings.Join(out, ", ")
	}

	config := func(n int) string {
		return testProviderConfig(srv.URL) + fmt.Sprintf(`
resource "omada_lan_dns" "test" {
  name            = "nas"
  domain          = "nas.example.internal"
  aliases         = [%s]
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = ["net-1"]
}`, aliasList(n))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// The over-limit step runs first so the run ends on a valid config —
			// the post-test destroy replays the last step, and replaying a config
			// that fails validation would fail the test for the wrong reason.
			{ // one over — rejected during plan, never reaching the controller
				Config:      config(maxLanDNSAliases + 1),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)aliases.*at most ` + fmt.Sprint(maxLanDNSAliases)),
			},
			{ // at the ceiling — still allowed
				Config: config(maxLanDNSAliases),
				Check: resource.TestCheckResourceAttr(
					"omada_lan_dns.test", "aliases.#", fmt.Sprint(maxLanDNSAliases),
				),
			},
		},
	})
}
