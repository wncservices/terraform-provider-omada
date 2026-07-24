// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSSHSettingsResource drives create → import → update of the SSH
// singleton (a PUT endpoint).
func TestAccSSHSettingsResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_ssh_settings" "this" {
  ssh_enable      = true
  ssh_server_port = 2222
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ssh_settings.this", "ssh_enable", "true"),
					resource.TestCheckResourceAttr("omada_ssh_settings.this", "ssh_server_port", "2222"),
					// Never set in config, so it comes back from the controller.
					resource.TestCheckResourceAttr("omada_ssh_settings.this", "layer3_access", "false"),
				),
			},
			{
				ResourceName:      "omada_ssh_settings.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_ssh_settings" "this" {
  ssh_enable      = false
  ssh_server_port = 22
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ssh_settings.this", "ssh_enable", "false"),
					resource.TestCheckResourceAttr("omada_ssh_settings.this", "ssh_server_port", "22"),
				),
			},
		},
	})
}
