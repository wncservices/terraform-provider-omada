// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccRateLimitProfileResource drives create → import → update.
//
// The subtlety is that a limit exists on the controller only while its enable
// flag is set: with the flag off the document omits the value entirely, so the
// attribute must come back null rather than zero.
func TestAccRateLimitProfileResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_rate_limit_profile" "guest" {
  name                  = "guest-capped"
  download_limit_enable = true
  download_limit        = 5000
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_rate_limit_profile.guest", "id"),
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "name", "guest-capped"),
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "download_limit", "5000"),
					// Upload is disabled, so the controller has no value and
					// the attribute stays null rather than becoming 0.
					resource.TestCheckNoResourceAttr("omada_rate_limit_profile.guest", "upload_limit"),
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "upload_limit_enable", "false"),
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "is_builtin", "false"),
				),
			},
			{
				ResourceName:      "omada_rate_limit_profile.guest",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_rate_limit_profile" "guest" {
  name                  = "guest-capped"
  download_limit_enable = true
  download_limit        = 20000
  upload_limit_enable   = true
  upload_limit          = 5000
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "download_limit", "20000"),
					resource.TestCheckResourceAttr("omada_rate_limit_profile.guest", "upload_limit", "5000"),
				),
			},
		},
	})
}

// TestAccRateLimitProfileValidation covers the plan-time guard: a limit
// without its enable flag would be silently dropped by the controller.
func TestAccRateLimitProfileValidation(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_rate_limit_profile" "bad" {
  name           = "no-enable"
  download_limit = 5000
}`,
				ExpectError: regexp.MustCompile(`download_limit requires download_limit_enable`),
			},
		},
	})
}
