// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccSessionLimitResource drives create → import → update.
//
// The property under test is that the per-host `table` in the same document is
// left alone: it must survive an update and must never be written back. The
// mock rejects a write containing it, so a regression fails here.
func TestAccSessionLimitResource(t *testing.T) {
	srv := newMockController(t)

	checkTablePreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["transmission/sessionLimits"]
		if _, ok := doc["table"]; !ok {
			return fmt.Errorf("the per-host rule table was dropped: %v", doc)
		}
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
resource "omada_session_limit" "this" {
  enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_session_limit.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_session_limit.this", "enable", "false"),
					// Never set in config: comes back from the controller.
					resource.TestCheckResourceAttr("omada_session_limit.this", "max_sessions", "128"),
					resource.TestCheckResourceAttr("omada_session_limit.this", "ip_session_enable", "true"),
					checkTablePreserved,
				),
			},
			{
				ResourceName:      "omada_session_limit.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_session_limit" "this" {
  enable       = true
  max_sessions = 512
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_session_limit.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_session_limit.this", "max_sessions", "512"),
					checkTablePreserved,
				),
			},
		},
	})
}
