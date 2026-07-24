// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccALGResource drives create → import → update of the ALG singleton.
// Beyond the usual round-trip it covers the int-list attributes (ftp_ports,
// sip_ports), which are the only list-typed fields in the singleton scaffold.
func TestAccALGResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["transmission/alg"]
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
resource "omada_alg" "this" {
  sip       = false
  ftp       = true
  ftp_ports = [21, 2121]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_alg.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_alg.this", "sip", "false"),
					resource.TestCheckResourceAttr("omada_alg.this", "ftp_ports.#", "2"),
					resource.TestCheckResourceAttr("omada_alg.this", "ftp_ports.1", "2121"),
					// Never set in config, so it comes back from the controller.
					resource.TestCheckResourceAttr("omada_alg.this", "sip_ports.#", "2"),
					resource.TestCheckResourceAttr("omada_alg.this", "pptp", "true"),
					checkUnmodelledPreserved,
				),
			},
			{
				ResourceName:      "omada_alg.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_alg" "this" {
  sip       = true
  ftp       = true
  ftp_ports = [21]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_alg.this", "sip", "true"),
					resource.TestCheckResourceAttr("omada_alg.this", "ftp_ports.#", "1"),
					checkUnmodelledPreserved,
				),
			},
		},
	})
}
