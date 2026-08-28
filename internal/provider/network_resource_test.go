// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccNetworkResource drives create → import → update of omada_network
// against the in-test mock, covering the per-VLAN toggles and DHCP options.
// Requires TF_ACC=1.
func TestAccNetworkResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]
  isolation      = true
  dhcp_enabled   = true
  dhcp_start     = "10.10.40.100"
  dhcp_end       = "10.10.40.200"
  dhcp_lease_time = 240
  dhcp_dns_mode   = "auto"

  dhcp_options = [
    { code = 138, type = 1, value = "10.10.20.50" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_network.test", "id"),
					resource.TestCheckResourceAttr("omada_network.test", "vlan_id", "40"),
					resource.TestCheckResourceAttr("omada_network.test", "purpose", "interface"),
					resource.TestCheckResourceAttr("omada_network.test", "isolation", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_lease_time", "240"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_dns_mode", "auto"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.#", "1"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.0.code", "138"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.0.value", "10.10.20.50"),
					resource.TestCheckResourceAttr("omada_network.test", "interface_ids.#", "2"),
					resource.TestCheckResourceAttr("omada_network.test", "site_id", "site-1"),
				),
			},
			{ResourceName: "omada_network.test", ImportState: true, ImportStateVerify: true},
			{ // update: flip isolation, change lease + a DHCP option
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]
  isolation      = false
  arp_detection_enable = true
  dhcp_enabled   = true
  dhcp_start     = "10.10.40.100"
  dhcp_end       = "10.10.40.200"
  dhcp_lease_time = 120
  dhcp_dns_mode   = "auto"

  dhcp_options = [
    { code = 138, type = 1, value = "10.10.99.5" },
    { code = 66,  type = 1, value = "10.10.20.60" },
  ]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_network.test", "isolation", "false"),
					resource.TestCheckResourceAttr("omada_network.test", "arp_detection_enable", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_lease_time", "120"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.#", "2"),
					resource.TestCheckResourceAttr("omada_network.test", "dhcp_options.1.code", "66"),
				),
			},
		},
	})
}

// TestAccNetworkIPv6RDNSS drives create -> import -> update of the ipv6 nested
// attribute in "Get from Prefix Delegation" mode (proto "rdnss"), including
// changing pre_id -- the real scenario is two networks sharing one WAN
// delegation via distinct pre_id values (e.g. 100 and 101).
func TestAccNetworkIPv6RDNSS(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]

  ipv6 = {
    enable = true
    proto  = "rdnss"
    rdnss = {
      pre_type  = 1
      port_uuid = "1_wan"
      pre_id    = 101
      dns_v6    = "auto"
    }
    ra = {
      enable             = true
      preference         = 1
      valid_lifetime     = 86400
      preferred_lifetime = 14400
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.enable", "true"),
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.proto", "rdnss"),
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.rdnss.pre_id", "101"),
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.rdnss.port_uuid", "1_wan"),
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.ra.valid_lifetime", "86400"),
				),
			},
			{ResourceName: "omada_network.test", ImportState: true, ImportStateVerify: true},
			{ // update: a second network would take pre_id 100 instead — exercise that change here
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_network" "test" {
  name           = "Lab"
  vlan_id        = 40
  gateway_subnet = "10.10.40.1/24"
  interface_ids  = ["port-2", "port-3"]

  ipv6 = {
    enable = true
    proto  = "rdnss"
    rdnss = {
      pre_type  = 1
      port_uuid = "1_wan"
      pre_id    = 100
      dns_v6    = "auto"
    }
    ra = {
      enable             = true
      preference         = 1
      valid_lifetime     = 86400
      preferred_lifetime = 14400
    }
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_network.test", "ipv6.rdnss.pre_id", "100"),
				),
			},
		},
	})
}

// TestAccNetworkCreateRequiresOpenAPI pins the error a practitioner meets when
// they add a new network with only the admin credentials configured.
//
// Import, read, update and delete all work without Open API credentials, so
// this failure appears only when someone adds their first network — long after
// the provider was set up and working. The message has to say which credentials
// are missing and where they come from, or it reads as the admin password
// having broken.
func TestAccNetworkCreateRequiresOpenAPI(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_network" "new" {
  name           = "LAB"
  vlan_id        = 77
  gateway_subnet = "10.10.77.1/24"
}`,
				ExpectError: regexp.MustCompile(`Platform Integration -> Open API`),
			},
		},
	})
}

// TestAccNetworkCreateRequiresInterfaces covers the constraint that only shows
// up once the VLAN id is valid: the controller refuses a network bound to no
// LAN interface (-33515). It cannot be deferred to the follow-up update the way
// the rest of the configuration is, so create has to check it.
func TestAccNetworkCreateRequiresInterfaces(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfigOpenAPI(srv.URL) + `
resource "omada_network" "new" {
  name           = "LAB"
  vlan_id        = 77
  gateway_subnet = "10.10.77.1/24"
  interface_ids  = []
}`,
				ExpectError: regexp.MustCompile(`at least one LAN interface`),
			},
		},
	})
}
