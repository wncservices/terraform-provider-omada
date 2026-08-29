// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWANIPv6Resource drives create -> import -> update of omada_wan_ipv6
// against the mock, covering the "dynamic" (prefix delegation) shape. Requires
// TF_ACC=1.
func TestAccWANIPv6Resource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_wan_ipv6" "test" {
  port_uuid  = "wan-1"
  enable     = true
  proto      = "dynamic"
  proto_type = 1

  dynamic = {
    get_ipv6      = "auto"
    get_ipv6_type = 3
    prefix        = 1
    pd_size       = 48
    dns           = "dynamic"
    dns_type      = 0
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_wan_ipv6.test", "id"),
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "enable", "true"),
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "proto", "dynamic"),
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "dynamic.pd_size", "48"),
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "dynamic.prefix", "1"),
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_wan_ipv6.test", ImportState: true, ImportStateVerify: true},
			{ // update: shrink the requested delegation size
				Config: testProviderConfig(srv.URL) + `
resource "omada_wan_ipv6" "test" {
  port_uuid  = "wan-1"
  enable     = true
  proto      = "dynamic"
  proto_type = 1

  dynamic = {
    get_ipv6      = "auto"
    get_ipv6_type = 3
    prefix        = 1
    pd_size       = 56
    dns           = "dynamic"
    dns_type      = 0
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_wan_ipv6.test", "dynamic.pd_size", "56"),
				),
			},
		},
	})
}

// TestAccWANIPv6ResourceIPv4Preserved confirms the read-modify-write in
// UpdateWANIPv6Setting never touches wanPortIpv4Setting/wanPortMacSetting —
// the entire reason it fetches the current document first instead of PATCHing
// a bare wanPortIpv6Setting object.
func TestAccWANIPv6ResourceIPv4Preserved(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_wan_ipv6" "test" {
  port_uuid = "wan-1"
  enable    = true
  proto     = "dynamic"
}
data "omada_wan" "after" {
  depends_on = [omada_wan_ipv6.test]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.omada_wan.after", "ports.0.proto", "dhcp"),
					resource.TestCheckResourceAttr("data.omada_wan.after", "ports.0.mac_method", "recover"),
					resource.TestCheckResourceAttr("data.omada_wan.after", "ports.0.ipv6_enable", "1"),
				),
			},
		},
	})
}
