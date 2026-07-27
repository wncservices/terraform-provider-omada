// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccGatewayResource drives adopt → import → update of the gateway.
//
// The property under test is that this resource touches only what it is told
// to. The gateway is the device the whole site routes through, so an update
// that reverted an unrelated setting — or, worse, wrote the physical port
// configuration — is the failure that matters. Both are asserted: the first
// step sets only `name` and checks the rest survives, and the mock rejects any
// body containing `portConfigs`.
func TestAccGatewayResource(t *testing.T) {
	srv := newMockController(t)

	checkUntouched := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "gateway")["gateway"]
		if got, _ := doc["unmodelledKey"].(string); got != "keep-me" {
			return fmt.Errorf("an unmodelled key was overwritten: %v", doc["unmodelledKey"])
		}
		ports, _ := doc["portConfigs"].([]any)
		if len(ports) != 1 {
			return fmt.Errorf("the gateway's port configuration was modified: %v", doc["portConfigs"])
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_gateway" "this" {
  mac  = "f0:09:0d:d0:97:76"
  name = "Mordor"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway.this", "id", "F0-09-0D-D0-97-76"),
					resource.TestCheckResourceAttr("omada_gateway.this", "name", "Mordor"),
					resource.TestCheckResourceAttr("omada_gateway.this", "model", "ER707-M2"),
					resource.TestCheckResourceAttr("omada_gateway.this", "site_id", "site-1"),
					// Not configured above, so read back as they were.
					resource.TestCheckResourceAttr("omada_gateway.this", "led_setting", "2"),
					resource.TestCheckResourceAttr("omada_gateway.this", "lldp_enable", "true"),
					resource.TestCheckResourceAttr("omada_gateway.this", "hw_offload_enable", "true"),
					resource.TestCheckResourceAttr("omada_gateway.this", "igmp_enable", "true"),
					resource.TestCheckResourceAttr("omada_gateway.this", "igmp_version", "2"),
					resource.TestCheckResourceAttr("omada_gateway.this", "supports_ip_passthrough", "false"),
					checkUntouched,
				),
			},
			{
				ResourceName:      "omada_gateway.this",
				ImportState:       true,
				ImportStateId:     "F0-09-0D-D0-97-76",
				ImportStateVerify: true,
				// State keeps the config's colon spelling; import reads the
				// controller's dashes. Same textual-comparison limitation as
				// omada_switch_port.
				ImportStateVerifyIgnore: []string{"mac"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_gateway" "this" {
  mac               = "f0:09:0d:d0:97:76"
  name              = "Mordor"
  lldp_enable       = false
  hw_offload_enable = true
  snmp_location     = "rack 1"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_gateway.this", "lldp_enable", "false"),
					resource.TestCheckResourceAttr("omada_gateway.this", "snmp_location", "rack 1"),
					// snmpSeting is sent whole, so the half that was not
					// configured must come from the device rather than being
					// blanked.
					resource.TestCheckResourceAttr("omada_gateway.this", "snmp_contact", ""),
					// Still untouched by an update that never mentioned them.
					resource.TestCheckResourceAttr("omada_gateway.this", "led_setting", "2"),
					resource.TestCheckResourceAttr("omada_gateway.this", "igmp_enable", "true"),
					checkUntouched,
				),
			},
		},
	})
}
