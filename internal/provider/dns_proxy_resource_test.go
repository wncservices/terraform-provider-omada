// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDNSProxyResource drives create → import → update of the DNS proxy.
//
// The property under test is the split ownership of the two server lists: the
// firmware's list may only have its flags flipped, while the customised list is
// replaced wholesale. The mock rejects a write that adds to or renumbers the
// firmware list, so a provider that rebuilt it from configuration fails here.
func TestAccDNSProxyResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dns_proxy" "this" {
  enable                       = true
  enabled_default_server_types = [1]

  custom_server {
    name = "filtered"
    urls = ["https://dns.example.com/dns-query"]
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "id", "site-1"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "enable", "true"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "enabled_default_server_types.#", "1"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "enabled_default_server_types.0", "1"),
					// The firmware list is reported in full, with the flag moved
					// onto exactly the requested entry.
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.#", "3"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.0.enabled", "false"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.1.type", "1"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.1.enabled", "true"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.#", "1"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.0.name", "filtered"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.0.enable", "true"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.0.urls.0", "https://dns.example.com/dns-query"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "doh_server_limit", "32"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "supports_dns_override", "true"),
				),
			},
			{
				ResourceName:      "omada_dns_proxy.this",
				ImportState:       true,
				ImportStateId:     "Default",
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dns_proxy" "this" {
  enable                       = true
  enabled_default_server_types = [0, 5]

  custom_server {
    name   = "filtered"
    urls   = ["https://dns.example.com/dns-query", "https://dns2.example.com/dns-query"]
  }

  custom_server {
    name   = "staged"
    enable = false
    urls   = ["https://example.invalid/dns-query"]
  }
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "enabled_default_server_types.#", "2"),
					// Flags moved off type 1 and onto 0 and 5, list intact.
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.#", "3"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.0.enabled", "true"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.1.enabled", "false"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "available_default_servers.2.enabled", "true"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.#", "2"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.0.urls.#", "2"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.1.name", "staged"),
					resource.TestCheckResourceAttr("omada_dns_proxy.this", "custom_server.1.enable", "false"),
				),
			},
		},
	})
}

// TestAccDNSProxyRejectsUnknownDefaultType covers a resolver type the firmware
// does not offer.
//
// The controller accepts such a write and drops it, so without this check a
// configuration could claim a resolver is enabled when it never will be — and
// for DNS that is a silent failure to route queries where you think they go.
func TestAccDNSProxyRejectsUnknownDefaultType(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dns_proxy" "this" {
  enable                       = true
  enabled_default_server_types = [99]
}`,
				ExpectError: regexp.MustCompile(`not one this firmware offers`),
			},
		},
	})
}

// TestAccDNSProxyRejectsEmptyURLs covers a customised resolver with no endpoint.
func TestAccDNSProxyRejectsEmptyURLs(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dns_proxy" "this" {
  enable = true

  custom_server {
    name = "broken"
    urls = []
  }
}`,
				ExpectError: regexp.MustCompile(`has no urls`),
			},
		},
	})
}
