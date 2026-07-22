// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccPortProfileResource drives create → import → update of
// omada_port_profile, including spanning-tree settings. Requires TF_ACC=1.
//
// It also asserts that controller-owned keys the provider never models — the STP
// `instances` list and `prohibitModify` — survive an update, proving the
// read-modify-write deep-merge works.
func TestAccPortProfileResource(t *testing.T) {
	srv := newMockController(t)

	checkPreserved := func(*terraform.State) error {
		for _, p := range rawStore(t, srv.URL, "profiles") {
			if _, ok := p["prohibitModify"]; !ok {
				return fmt.Errorf("prohibitModify was dropped from the profile: %v", p)
			}
			stp, _ := p["spanningTreeSetting"].(map[string]any)
			if stp == nil {
				return fmt.Errorf("spanningTreeSetting missing: %v", p)
			}
			if _, ok := stp["instances"]; !ok {
				return fmt.Errorf("STP instances were dropped by the update: %v", stp)
			}
		}
		return nil
	}

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
  lldp_med_enable    = true
  dot1x              = 2
  spanning_tree_enable = false
  stp_priority         = 128
  stp_bpdu_forward     = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_port_profile.test", "id"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "poe", "2"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "lldp_med_enable", "true"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "dot1x", "2"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "stp_priority", "128"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "stp_bpdu_forward", "true"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "tagged_network_ids.#", "2"),
					checkPreserved,
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
  lldp_med_enable    = false
  dot1x              = 0
  spanning_tree_enable = true
  stp_priority         = 64
  stp_edge_port        = true
  stp_bpdu_forward     = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_port_profile.test", "poe", "0"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "lldp_med_enable", "false"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "spanning_tree_enable", "true"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "stp_priority", "64"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "stp_edge_port", "true"),
					resource.TestCheckResourceAttr("omada_port_profile.test", "tagged_network_ids.#", "1"),
					checkPreserved,
				),
			},
		},
	})
}
