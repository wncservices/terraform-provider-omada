// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccQoSBandwidthControlResource drives create → import → update.
//
// Create is the interesting path: the controller returns a **null** result, so
// the client has to resolve the new rule from the list by its WAN. That only
// works because one rule per WAN port is permitted, which the mock enforces
// the same way the controller does.
func TestAccQoSBandwidthControlResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_qos_bandwidth_control" "wan" {
  wan           = "1_c967cf39292e474291e409b4dfe7f0cd"
  direction     = 2
  in_bandwidth  = 1000000
  out_bandwidth = 1000000
  class_ratio   = [25, 25, 25, 25]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_qos_bandwidth_control.wan", "id"),
					// Staged disabled by default so it cannot shape traffic
					// until that is an explicit choice.
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "enable", "false"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "class_ratio.#", "4"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "in_bandwidth", "1000000"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_qos_bandwidth_control.wan",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_qos_bandwidth_control" "wan" {
  wan                   = "1_c967cf39292e474291e409b4dfe7f0cd"
  enable                = true
  direction             = 2
  in_bandwidth          = 900000
  out_bandwidth         = 500000
  class_ratio           = [40, 30, 20, 10]
  udp_bandwidth_control = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "enable", "true"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "in_bandwidth", "900000"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "class_ratio.0", "40"),
					resource.TestCheckResourceAttr("omada_qos_bandwidth_control.wan", "udp_bandwidth_control", "true"),
				),
			},
		},
	})
}

// TestAccQoSBandwidthControlClassRatio covers the plan-time validation. Both
// rules were confirmed against a real controller, which rejects either with a
// bare -1001; catching them during planning gives a usable message instead.
func TestAccQoSBandwidthControlClassRatio(t *testing.T) {
	srv := newMockController(t)

	base := testProviderConfig(srv.URL) + `
resource "omada_qos_bandwidth_control" "wan" {
  wan           = "1_c967cf39292e474291e409b4dfe7f0cd"
  direction     = 2
  in_bandwidth  = 1000000
  out_bandwidth = 1000000
  class_ratio   = %s
}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(base, "[34, 33, 33]"),
				ExpectError: regexp.MustCompile(`exactly four entries`),
			},
			{
				Config:      fmt.Sprintf(base, "[25, 25, 25, 15]"),
				ExpectError: regexp.MustCompile(`must total 100`),
			},
		},
	})
}
