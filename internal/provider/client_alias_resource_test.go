// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccClientAliasResource drives adopt -> import -> update. The mock rejects
// any PATCH body containing more than name, so the test also proves that client
// runtime and unrelated persistent settings are never round-tripped.
func TestAccClientAliasResource(t *testing.T) {
	srv := newMockController(t)

	checkClient := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			client := rawStore(t, srv.URL, "clients")["00-11-22-33-44-55"]
			if got, _ := client["name"].(string); got != want {
				return fmt.Errorf("client alias = %q, want %q", got, want)
			}
			if _, ok := client["ipSetting"]; !ok {
				return fmt.Errorf("client ipSetting was removed")
			}
			if _, ok := client["rateLimit"]; !ok {
				return fmt.Errorf("client rateLimit was removed")
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClient("Office Printer"),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00:11:22:33:44:55"
  alias = "Printer"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_client_alias.printer", "id", "00-11-22-33-44-55"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "mac", "00:11:22:33:44:55"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "alias", "Printer"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "site", "Default"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "site_id", "site-1"),
					checkClient("Printer"),
				),
			},
			{
				ResourceName:            "omada_client_alias.printer",
				ImportState:             true,
				ImportStateId:           "00-11-22-33-44-55",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"mac"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00:11:22:33:44:55"
  alias = "Office Printer"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_client_alias.printer", "alias", "Office Printer"),
					checkClient("Office Printer"),
				),
			},
		},
	})
}
