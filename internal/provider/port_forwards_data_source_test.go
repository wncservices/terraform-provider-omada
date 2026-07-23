// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccPortForwardsDataSource lists port forwards off the mock controller,
// which is seeded with one rule (pf-seed). Requires TF_ACC=1.
func TestAccPortForwardsDataSource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `data "omada_port_forwards" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "site_id", "site-1"),
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "port_forwards.#", "1"),
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "port_forwards.0.id", "pf-seed"),
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "port_forwards.0.name", "seed"),
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "port_forwards.0.enabled", "true"),
					resource.TestCheckResourceAttr("data.omada_port_forwards.test", "port_forwards.0.forward_ip", "10.10.20.9"),
				),
			},
		},
	})
}
