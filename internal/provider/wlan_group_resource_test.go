// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWLANGroupResource drives create → import → update of omada_wlan_group
// against the in-test mock. Requires TF_ACC=1.
func TestAccWLANGroupResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `resource "omada_wlan_group" "test" { name = "IoT-Group" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_wlan_group.test", "id"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "name", "IoT-Group"),
					resource.TestCheckResourceAttr("omada_wlan_group.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_wlan_group.test", ImportState: true, ImportStateVerify: true},
			{
				Config: testProviderConfig(srv.URL) + `resource "omada_wlan_group" "test" { name = "IoT-Group-2" }`,
				Check:  resource.TestCheckResourceAttr("omada_wlan_group.test", "name", "IoT-Group-2"),
			},
		},
	})
}
