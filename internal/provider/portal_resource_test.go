// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccPortalResource drives create → import → update of omada_portal.
//
// It also asserts the two properties that matter most for a captive portal:
// the write-only password is actually sent to the controller but never lands in
// Terraform state, and the controller-owned landing-page design
// (portalCustomize) survives an update. Requires TF_ACC=1.
func TestAccPortalResource(t *testing.T) {
	srv := newMockController(t)

	// The password must reach the controller...
	checkPasswordStored := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			for _, p := range rawStore(t, srv.URL, "portals") {
				sp, _ := p["simplePassword"].(map[string]any)
				if sp == nil {
					return fmt.Errorf("simplePassword missing from stored portal: %v", p)
				}
				if got, _ := sp["password"].(string); got != want {
					return fmt.Errorf("stored password = %q, want %q", got, want)
				}
			}
			return nil
		}
	}
	// ...and the landing-page design must not be clobbered by an update.
	checkCustomizePreserved := func(*terraform.State) error {
		for _, p := range rawStore(t, srv.URL, "portals") {
			pc, _ := p["portalCustomize"].(map[string]any)
			if pc == nil {
				return fmt.Errorf("portalCustomize was dropped: %v", p)
			}
			if _, ok := pc["defaultLanguage"]; !ok {
				return fmt.Errorf("portalCustomize.defaultLanguage was dropped: %v", pc)
			}
			if got, _ := pc["buttonText"].(string); got != "Log In" {
				return fmt.Errorf("portalCustomize.buttonText = %q, want the controller default", got)
			}
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_portal" "guest" {
  name        = "Guest Portal"
  auth_type   = 1
  password    = "s3cret-guest"
  ssid_ids    = ["ssid-guest"]
  network_ids = []
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkSecretsAbsentFromState(t, "s3cret-guest", "rotated-pw"),
					resource.TestCheckResourceAttrSet("omada_portal.guest", "id"),
					resource.TestCheckResourceAttr("omada_portal.guest", "enable", "true"),
					resource.TestCheckResourceAttr("omada_portal.guest", "auth_type", "1"),
					resource.TestCheckResourceAttr("omada_portal.guest", "ssid_ids.#", "1"),
					resource.TestCheckResourceAttr("omada_portal.guest", "ssid_ids.0", "ssid-guest"),
					resource.TestCheckResourceAttr("omada_portal.guest", "site_id", "site-1"),
					checkPasswordStored("s3cret-guest"),
					checkCustomizePreserved,
				),
			},
			{
				// password is write-only: the controller never returns it, so it
				// cannot round-trip through import.
				ResourceName:            "omada_portal.guest",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_portal" "guest" {
  name        = "Guest Portal"
  auth_type   = 1
  password    = "rotated-pw"
  enable      = false
  ssid_ids    = ["ssid-guest"]
  network_ids = []
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkSecretsAbsentFromState(t, "s3cret-guest", "rotated-pw"),
					resource.TestCheckResourceAttr("omada_portal.guest", "enable", "false"),
					checkPasswordStored("rotated-pw"),
					checkCustomizePreserved,
				),
			},
		},
	})
}
