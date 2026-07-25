// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccTimeRangeResource drives create → import → update of omada_time_range,
// including adding a second window on update.
//
// The case worth protecting here is the controller-assigned `ruleId` it stamps
// onto every slot: the provider must read a slot back without treating that
// extra key as a change, or every plan would show a permanent diff.
func TestAccTimeRangeResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_time_range" "night" {
  name      = "night run"
  monday    = true
  tuesday   = true
  wednesday = true
  thursday  = true
  friday    = true

  time_slots {
    start_hour = 3
    end_hour   = 6
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_time_range.night", "id"),
					resource.TestCheckResourceAttr("omada_time_range.night", "name", "night run"),
					resource.TestCheckResourceAttr("omada_time_range.night", "monday", "true"),
					resource.TestCheckResourceAttr("omada_time_range.night", "saturday", "false"),
					resource.TestCheckResourceAttr("omada_time_range.night", "site_id", "site-1"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.#", "1"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.0.start_hour", "3"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.0.start_minute", "0"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.0.end_hour", "6"),
				),
			},
			{
				ResourceName:      "omada_time_range.night",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_time_range" "night" {
  name      = "night run (extended)"
  monday    = true
  tuesday   = true
  wednesday = true
  thursday  = true
  friday    = true
  saturday  = true

  time_slots {
    start_hour   = 3
    start_minute = 30
    end_hour     = 6
  }

  time_slots {
    start_hour = 13
    end_hour   = 14
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_time_range.night", "name", "night run (extended)"),
					resource.TestCheckResourceAttr("omada_time_range.night", "saturday", "true"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.#", "2"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.0.start_minute", "30"),
					resource.TestCheckResourceAttr("omada_time_range.night", "time_slots.1.start_hour", "13"),
				),
			},
		},
	})
}
