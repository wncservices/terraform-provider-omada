// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDevicesDataSource lists adopted devices off the mock controller, which
// returns a bare array (no pagination envelope) seeded with a switch and an AP.
// Requires TF_ACC=1.
func TestAccDevicesDataSource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `data "omada_devices" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_devices.test", "site_id", "site-1"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.#", "2"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.0.name", "SW-1"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.0.type", "switch"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.0.model", "ES205GP"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.0.status_category", "1"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.0.uptime_seconds", "5656402"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.1.type", "ap"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.1.need_upgrade", "true"),
					resource.TestCheckResourceAttr("data.omada_devices.test", "devices.1.client_num", "7"),
				),
			},
		},
	})
}
