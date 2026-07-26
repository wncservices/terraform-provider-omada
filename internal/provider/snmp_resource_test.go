// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccSNMPResource drives create → import → update of the SNMP singleton.
//
// The point of this test is the write-only credentials. The controller returns
// the v3 password in plaintext on every read, so a provider that simply mapped
// the field would copy it into state. These checks assert it reaches the
// controller, survives an update that does not re-supply it, and is absent
// from state throughout.
func TestAccSNMPResource(t *testing.T) {
	srv := newMockController(t)

	storedSecret := func(key string) string {
		doc := rawStore(t, srv.URL, "singletons")["snmp"]
		v, _ := doc[key].(string)
		return v
	}
	checkStored := func(key, want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			if got := storedSecret(key); got != want {
				return fmt.Errorf("stored %s = %q, want %q", key, got, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_snmp" "this" {
  v3_enable   = true
  v3_username = "monitoring"
  v3_password = "sup3r-secret-v3"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_snmp.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_snmp.this", "v3_enable", "true"),
					resource.TestCheckResourceAttr("omada_snmp.this", "v3_username", "monitoring"),
					// Never set in config: comes back from the controller.
					resource.TestCheckResourceAttr("omada_snmp.this", "auth_mode", "1"),
					checkStored("password", "sup3r-secret-v3"),
					checkSecretsAbsentFromState(t, "sup3r-secret-v3", "seeded-v3-password", "rotated-v3"),
				),
			},
			{
				ResourceName:      "omada_snmp.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				// Renames the v3 user without re-supplying the password: the
				// controller must still hold the original secret afterwards.
				Config: testProviderConfig(srv.URL) + `
resource "omada_snmp" "this" {
  v3_enable   = true
  v3_username = "monitoring-renamed"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_snmp.this", "v3_username", "monitoring-renamed"),
					checkStored("password", "sup3r-secret-v3"),
					checkSecretsAbsentFromState(t, "sup3r-secret-v3", "rotated-v3"),
				),
			},
		},
	})
}
