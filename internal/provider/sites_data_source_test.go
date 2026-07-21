// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSitesDataSource runs the omada_sites data source against the in-test
// mock controller. Requires TF_ACC=1 (Terraform acceptance convention); no real
// controller or secrets needed, so it runs in CI.
func TestAccSitesDataSource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `data "omada_sites" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.#", "1"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.id", "site-1"),
					resource.TestCheckResourceAttr("data.omada_sites.test", "sites.0.name", "Default"),
				),
			},
		},
	})
}
