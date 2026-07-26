// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccPortalAccessControlResource drives create → import → update, and
// asserts the unmodelled policy lists survive: this resource manages only the
// two switches, so wiping the policies behind them would be a silent loss.
func TestAccPortalAccessControlResource(t *testing.T) {
	srv := newMockController(t)

	checkPoliciesPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["accessControl"]
		for _, k := range []string{"preAuthAccessPolicies", "freeAuthClientPolicies"} {
			if _, ok := doc[k]; !ok {
				return fmt.Errorf("policy list %q was dropped: %v", k, doc)
			}
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_portal_access_control" "this" {
  pre_auth_access_enable  = false
  free_auth_client_enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_portal_access_control.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_portal_access_control.this", "pre_auth_access_enable", "false"),
					checkPoliciesPreserved,
				),
			},
			{
				ResourceName:      "omada_portal_access_control.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_portal_access_control" "this" {
  pre_auth_access_enable  = true
  free_auth_client_enable = true
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_portal_access_control.this", "pre_auth_access_enable", "true"),
					resource.TestCheckResourceAttr("omada_portal_access_control.this", "free_auth_client_enable", "true"),
					checkPoliciesPreserved,
				),
			},
		},
	})
}
