// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMDNSProfileResource drives create -> import -> update of a custom
// mDNS service profile, and checks the two things that distinguish this
// endpoint from omada_mdns_reflector: create answers with the id as a bare
// string rather than an object, and `is_builtin` is reported but never sent.
func TestAccMDNSProfileResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mdns_profile" "matter" {
  name        = "matter"
  service_ids = ["_matter._tcp.local", "_matterc._udp.local"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_mdns_profile.matter", "id"),
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "name", "matter"),
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "service_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "service_ids.0", "_matter._tcp.local"),
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "service_ids.1", "_matterc._udp.local"),
					// A custom profile is never a built-in.
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "is_builtin", "false"),
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_mdns_profile.matter",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mdns_profile" "matter" {
  name        = "matter"
  service_ids = ["_matter._tcp.local", "_matterc._udp.local", "_http._tcp.local"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_mdns_profile.matter", "service_ids.#", "3"),
				),
			},
		},
	})
}
