// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccIoTServerResource drives create → import → update of an IoT telemetry
// server.
//
// Two things are specific to this resource. `filters` is a controller-owned key
// whose populated shape has never been observed, so it must survive an update
// untouched — the mock seeds it and the check below asserts it is still there.
// And `ssl_tls_enable` is nested under `sslTls` on the wire, so it exercises the
// flat-attribute/nested-document translation in both directions.
func TestAccIoTServerResource(t *testing.T) {
	srv := newMockController(t)

	checkFiltersPreserved := func(*terraform.State) error {
		for _, doc := range rawStore(t, srv.URL, "iotServers") {
			f, ok := doc["filters"].(map[string]any)
			if !ok || f["seeded"] != true {
				return fmt.Errorf("controller-owned filters were dropped: %v", doc)
			}
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_server" "telemetry" {
  name       = "test"
  server_url = "https://telemetry.example.com/ingest"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_iot_server.telemetry", "id"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "name", "test"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "site_id", "site-1"),
					// Staged off, and TLS on, by default — both deliberate.
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "enable", "false"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "ssl_tls_enable", "true"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "authentication", "99"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "count_only", "false"),
					checkFiltersPreserved,
				),
			},
			{
				ResourceName:      "omada_iot_server.telemetry",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_server" "telemetry" {
  name            = "test (renamed)"
  server_url      = "https://telemetry.example.com/v2"
  enable          = true
  count_only      = true
  raw_data        = false
  device_classes  = [0, 1]
  report_interval = 5
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "name", "test (renamed)"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "server_url", "https://telemetry.example.com/v2"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "enable", "true"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "count_only", "true"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "report_interval", "5"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "device_classes.#", "2"),
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "device_classes.1", "1"),
					// Still on after an update that did not mention it.
					resource.TestCheckResourceAttr("omada_iot_server.telemetry", "ssl_tls_enable", "true"),
					checkFiltersPreserved,
				),
			},
		},
	})
}

// TestAccIoTServerRejectsEmptyDeviceClasses covers the one create constraint
// that is not obvious from the controller's own message.
//
// It answers "The parameter Device Class is required" for an empty list, which
// reads as a missing field rather than an empty one — so the provider says
// which attribute and why instead of passing that through.
func TestAccIoTServerRejectsEmptyDeviceClasses(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_server" "telemetry" {
  name           = "test"
  server_url     = "https://telemetry.example.com/ingest"
  device_classes = []
}`,
				ExpectError: regexp.MustCompile(`device_classes must list`),
			},
		},
	})
}
