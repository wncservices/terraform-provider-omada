// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMDNSReflectorResource drives create → import → update of
// omada_mdns_reflector against the in-test mock. Requires TF_ACC=1.
func TestAccMDNSReflectorResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mdns_reflector" "test" {
  name         = "Media to Main"
  profile_ids  = ["buildIn-1"]
  service_vlan = "40"
  client_vlan  = "10"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_mdns_reflector.test", "id"),
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "service_vlan", "40"),
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "client_vlan", "10"),
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "enable", "true"),
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "profile_ids.0", "buildIn-1"),
				),
			},
			{ResourceName: "omada_mdns_reflector.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mdns_reflector" "test" {
  name         = "Media to Main"
  enable       = false
  profile_ids  = ["buildIn-1"]
  service_vlan = "40"
  client_vlan  = "20"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "client_vlan", "20"),
					resource.TestCheckResourceAttr("omada_mdns_reflector.test", "enable", "false"),
				),
			},
		},
	})
}
