// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIPTVResource drives create → import → update of the IPTV singleton.
//
// The property worth protecting is that the port list is the controller's, not
// Terraform's: the resource may flip each row's flag but must never invent,
// drop or rename a row. The mock rejects a write whose port set does not match,
// so a provider that rebuilt the list from configuration would fail here.
func TestAccIPTVResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iptv" "this" {
  igmp_proxy_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iptv.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_iptv.this", "igmp_proxy_enable", "true"),
					resource.TestCheckResourceAttr("omada_iptv.this", "igmp_version", "2"),
					// Off by default, and no port switched into IPTV mode.
					resource.TestCheckResourceAttr("omada_iptv.this", "enable", "false"),
					resource.TestCheckResourceAttr("omada_iptv.this", "enabled_port_ids.#", "0"),
					// The controller's port list is reported, not managed.
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.#", "2"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.0.id", "3_aaa"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.0.name", "WAN/LAN3"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.0.support_iptv", "true"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.0.enabled", "false"),
				),
			},
			{
				ResourceName:      "omada_iptv.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iptv" "this" {
  igmp_proxy_enable = true
  igmp_version      = "3"
  enable            = true
  enabled_port_ids  = ["4_bbb"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iptv.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_iptv.this", "igmp_version", "3"),
					resource.TestCheckResourceAttr("omada_iptv.this", "enabled_port_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_iptv.this", "enabled_port_ids.0", "4_bbb"),
					// The flag moved on the right row, and only that row.
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.#", "2"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.0.enabled", "false"),
					resource.TestCheckResourceAttr("omada_iptv.this", "available_ports.1.enabled", "true"),
				),
			},
		},
	})
}

// TestAccIPTVRejectsUnknownPort covers a port id that is not one of this
// gateway's — almost always a stale id copied from another site.
//
// The controller would take such a write and silently do nothing with it, so
// the check is the provider's: a config that says a port is in IPTV mode when
// it never will be is worse than an error.
func TestAccIPTVRejectsUnknownPort(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iptv" "this" {
  enable           = true
  enabled_port_ids = ["9_nope"]
}`,
				ExpectError: regexp.MustCompile(`not one of this gateway's ports`),
			},
		},
	})
}
