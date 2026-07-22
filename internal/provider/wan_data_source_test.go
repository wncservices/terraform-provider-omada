// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWANDataSource reads the gateway WAN ports off the mock controller and
// asserts the flattening of the nested wanPortIpv4Setting / wanPortMacSetting
// sub-objects. Requires TF_ACC=1.
func TestAccWANDataSource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `data "omada_wan" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_wan.test", "site_id", "site-1"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.#", "1"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.port_uuid", "wan-1"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.port_name", "WAN/LAN1"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.proto", "dhcp"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.vlan_id", "0"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.qos_tag_enable", "false"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.mtu", "1500"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.mac_method", "recover"),
					resource.TestCheckResourceAttr("data.omada_wan.test", "ports.0.ipv6_enable", "0"),
				),
			},
		},
	})
}
