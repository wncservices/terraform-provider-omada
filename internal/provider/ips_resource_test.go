// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccIPSResource drives create → import → update of the IPS singleton.
//
// The distinctive property here is the split between configuration and
// controller-owned reference data: the *_categories lists describing each
// protection level must be reported to the practitioner but never written
// back. The mock rejects a write containing one, so a regression that starts
// sending them fails here rather than against a real gateway.
func TestAccIPSResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["ips"]
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
resource "omada_ips" "this" {
  enable           = true
  geo_blocking     = true
  protection_level = 3
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ips.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_ips.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_ips.this", "protection_level", "3"),
					// Never set in config: comes back from the controller.
					resource.TestCheckResourceAttr("omada_ips.this", "ips_mode", "1"),
					// Read-only reference data is reported...
					resource.TestCheckResourceAttr("omada_ips.this", "low_categories.#", "2"),
					resource.TestCheckResourceAttr("omada_ips.this", "high_categories.#", "4"),
					resource.TestCheckResourceAttr("omada_ips.this", "all_categories.#", "4"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_ips.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_ips" "this" {
  enable            = true
  geo_blocking      = false
  protection_level  = 2
  custom_categories = [1, 2, 3]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_ips.this", "geo_blocking", "false"),
					resource.TestCheckResourceAttr("omada_ips.this", "protection_level", "2"),
					resource.TestCheckResourceAttr("omada_ips.this", "custom_categories.#", "3"),
					// ...and still intact after an update that never sends it.
					resource.TestCheckResourceAttr("omada_ips.this", "all_categories.#", "4"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
