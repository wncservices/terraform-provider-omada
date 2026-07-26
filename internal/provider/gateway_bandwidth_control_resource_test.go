// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccGatewayBandwidthControlResource covers the read/write asymmetry that
// motivated dotted keys: the controller returns these settings nested under
// `bandwidthControl` but accepts them flat, and rejects the nested form. The
// mock rejects it too, so sending the wrong shape fails here.
func TestAccGatewayBandwidthControlResource(t *testing.T) {
	srv := newMockController(t)

	checkPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "singletons")["transmission/bandwidthControls"]
		if _, ok := doc["table"]; !ok {
			return fmt.Errorf("per-host rule table was dropped: %v", doc)
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
resource "omada_gateway_bandwidth_control" "this" {
  enable = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway_bandwidth_control.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_gateway_bandwidth_control.this", "enable", "false"),
					// Read out of the nested object even though writes are flat.
					resource.TestCheckResourceAttr("omada_gateway_bandwidth_control.this", "threshold_percent", "80"),
					checkPreserved,
				),
			},
			{
				ResourceName:      "omada_gateway_bandwidth_control.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_gateway_bandwidth_control" "this" {
  enable            = true
  threshold_enable  = true
  threshold_percent = 90
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway_bandwidth_control.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_gateway_bandwidth_control.this", "threshold_percent", "90"),
					checkPreserved,
				),
			},
		},
	})
}
