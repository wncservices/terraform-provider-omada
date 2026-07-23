// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccFirewallACLsDataSource lists ACLs across all types off the mock
// controller, which is seeded with one gateway rule (acl-seed). Requires TF_ACC=1.
func TestAccFirewallACLsDataSource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `data "omada_firewall_acls" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "site_id", "site-1"),
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "acls.#", "1"),
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "acls.0.id", "acl-seed"),
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "acls.0.type", "gateway"),
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "acls.0.name", "seed"),
					resource.TestCheckResourceAttr("data.omada_firewall_acls.test", "acls.0.policy", "1"),
				),
			},
		},
	})
}
