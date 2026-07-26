// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccMACAuthResource drives create → import → update. It also exercises
// the scaffold's string-list kind, which this resource is the first to use.
func TestAccMACAuthResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["macAuth"]
		if got, _ := doc["unmodelledKey"].(string); got != "keep-me" {
			return fmt.Errorf("unmodelled key was dropped: %v", doc)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mac_auth" "this" {
  enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_mac_auth.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_mac_auth.this", "enable", "false"),
					// Never set in config: comes back from the controller.
					resource.TestCheckResourceAttr("omada_mac_auth.this", "auth_type", "0"),
					resource.TestCheckResourceAttr("omada_mac_auth.this", "ssid_ids.#", "0"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_mac_auth.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_mac_auth" "this" {
  enable    = true
  auth_type = 1
  ssid_ids  = ["ssid-guest", "ssid-iot"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_mac_auth.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_mac_auth.this", "auth_type", "1"),
					resource.TestCheckResourceAttr("omada_mac_auth.this", "ssid_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_mac_auth.this", "ssid_ids.1", "ssid-iot"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
