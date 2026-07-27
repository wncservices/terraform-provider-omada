// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccIoTRadioResource drives create → import → update of the IoT radio.
//
// Two things are specific to this resource. It is served only by the Open API,
// so the whole test runs on the credentialed provider config — including the
// import step, which for every other singleton needs nothing but the admin
// login. And `passcode` is write-only, so it must reach the controller and
// never appear in state.
func TestAccIoTRadioResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_iot_radio" "this" {
  enable         = true
  passcode       = "s3cr3t"
  transmit_power = 0
  aging_time     = 30
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iot_radio.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_iot_radio.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_iot_radio.this", "aging_time", "30"),
					// The configured passcode must not be persisted, and neither
					// must the one the mock seeds and hands back on every read.
					checkSecretsAbsentFromState(t, "s3cr3t", "seeded-passcode"),
				),
			},
			{
				ResourceName:      "omada_iot_radio.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
				// Write-only, so it is never in state to compare.
				ImportStateVerifyIgnore: []string{"passcode"},
			},
			{
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_iot_radio" "this" {
  enable         = false
  transmit_power = 2
  aging_time     = 60
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iot_radio.this", "enable", "false"),
					resource.TestCheckResourceAttr("omada_iot_radio.this", "transmit_power", "2"),
					resource.TestCheckResourceAttr("omada_iot_radio.this", "aging_time", "60"),
					checkSecretsAbsentFromState(t, "s3cr3t", "seeded-passcode"),
				),
			},
		},
	})
}

// TestAccIoTRadioRequiresOpenAPI covers the failure that makes this document
// different from every other singleton: without Open API credentials it cannot
// even be *read*, so the error appears on refresh rather than on apply.
func TestAccIoTRadioRequiresOpenAPI(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_radio" "this" {
  enable = true
}`,
				ExpectError: regexp.MustCompile(`Platform Integration -> Open API`),
			},
		},
	})
}
