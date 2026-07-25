// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccRadiusProfileResource drives create → import → update.
//
// The property under test is the write-only shared secret: it must reach the
// controller, must never appear in Terraform state, and — the part that is
// easy to get wrong — must survive an update that does not re-supply it,
// rather than being blanked.
func TestAccRadiusProfileResource(t *testing.T) {
	srv := newMockController(t)

	storedSecret := func(t *testing.T) string {
		t.Helper()
		for _, p := range rawStore(t, srv.URL, "radius") {
			servers, _ := p["authServer"].([]any)
			if len(servers) == 0 {
				continue
			}
			first, _ := servers[0].(map[string]any)
			pw, _ := first["radiusPwd"].(string)
			return pw
		}
		return ""
	}
	checkSecret := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := storedSecret(t); got != want {
				return fmt.Errorf("stored shared secret = %q, want %q", got, want)
			}
			return nil
		}
	}
	checkSecretNotInState := checkSecretsAbsentFromState(t, "s3cret-radius", "rotated-radius")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_radius_profile" "corp" {
  name                          = "corp-radius"
  require_message_authenticator = true

  auth_server {
    ip            = "10.10.99.5"
    shared_secret = "s3cret-radius"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_radius_profile.corp", "id"),
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "name", "corp-radius"),
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "require_message_authenticator", "true"),
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "auth_server.#", "1"),
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "auth_server.0.port", "1812"),
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "built_in_server", "false"),
					checkSecret("s3cret-radius"),
					checkSecretNotInState,
				),
			},
			{
				ResourceName:      "omada_radius_profile.corp",
				ImportState:       true,
				ImportStateVerify: true,
				// Write-only: the provider refuses to read secrets back, so
				// they cannot round-trip through import.
				ImportStateVerifyIgnore: []string{"auth_server.0.shared_secret"},
			},
			{
				// Renames the profile and does NOT re-supply the secret: the
				// controller must still hold the original one afterwards.
				Config: testProviderConfig(srv.URL) + `
resource "omada_radius_profile" "corp" {
  name                          = "corp-radius (renamed)"
  require_message_authenticator = true

  auth_server {
    ip = "10.10.99.5"
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_radius_profile.corp", "name", "corp-radius (renamed)"),
					checkSecret("s3cret-radius"),
					checkSecretNotInState,
				),
			},
		},
	})
}
