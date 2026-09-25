// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccFirmwareUpgradeScheduleResource drives create -> import -> update of
// omada_firmware_upgrade_schedule. The mock refuses a partial PATCH and has no
// GET by id, as the live controller does, so this also pins update to sending
// the whole schedule and read to going through the list. Requires TF_ACC=1.
func TestAccFirmwareUpgradeScheduleResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_firmware_upgrade_schedule" "test" {
  sites       = ["Default"]
  models      = ["EAP670(US) v2.0", "ES205G v1.20"]
  timing_type = 2
  hour        = 4
  minute      = 15
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_firmware_upgrade_schedule.test", "id"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "sites.#", "1"),
					resource.TestCheckTypeSetElemAttr("omada_firmware_upgrade_schedule.test", "sites.*", "Default"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "models.#", "2"),
					resource.TestCheckTypeSetElemAttr("omada_firmware_upgrade_schedule.test", "models.*", "ES205G v1.20"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "timing_type", "2"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "day_of_week", "0"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "day_of_month", "1"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "month_of_year", "1"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "channel", "0"),
				),
			},
			{ResourceName: "omada_firmware_upgrade_schedule.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_firmware_upgrade_schedule" "test" {
  sites        = ["Default"]
  models       = ["ES205G v1.20"]
  timing_type  = 3
  hour         = 5
  minute       = 8
  day_of_month = 2
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "models.#", "1"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "timing_type", "3"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "hour", "5"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "minute", "8"),
					resource.TestCheckResourceAttr("omada_firmware_upgrade_schedule.test", "day_of_month", "2"),
				),
			},
		},
	})
}

// TestAccFirmwareUpgradeScheduleResource_unknownModel checks that a model the
// sites do not have is refused before anything is written, with the models
// that are available in the message.
func TestAccFirmwareUpgradeScheduleResource_unknownModel(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_firmware_upgrade_schedule" "test" {
  sites       = ["Default"]
  models      = ["EAP245 v3.0"]
  timing_type = 1
  hour        = 3
}`,
				ExpectError: regexp.MustCompile(`"EAP245 v3.0" is not a model on the selected sites`),
			},
		},
	})
}
