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

// TestAccAccessPointResource drives adopt → update of an EAP.
//
// The property under test is not "did the attribute change" but "what did the
// provider send". On real hardware the controller restarts a radio whenever a
// radio document is written — even one whose values are identical — and every
// client on that band drops. So a resource that wrote radios unconditionally
// would take the band down on applies that change nothing about it.
//
// Both steps therefore assert the radio write count, not just the state.
func TestAccAccessPointResource(t *testing.T) {
	srv := newMockController(t)

	radioWrites := func(band string) int {
		store := rawStore(t, srv.URL, "accessPoint")
		n, _ := store["radioWrites"][band].(float64)
		return int(n)
	}

	noRadioWrites := func(*terraform.State) error {
		if got := radioWrites("radioSetting2g") + radioWrites("radioSetting5g"); got != 0 {
			return fmt.Errorf("a radio was restarted for a change that did not touch one: %d write(s)", got)
		}
		return nil
	}

	checkUntouched := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "accessPoint")["accessPoint"]
		if got, _ := doc["unmodelledKey"].(string); got != "keep-me" {
			return fmt.Errorf("an unmodelled key was overwritten: %v", doc["unmodelledKey"])
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Naming the AP must not disturb its radios.
				Config: testProviderConfig(srv.URL) + `
resource "omada_access_point" "this" {
  mac  = "10:5a:95:89:c3:64"
  name = "Living Room"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_access_point.this", "id", "10-5A-95-89-C3-64"),
					resource.TestCheckResourceAttr("omada_access_point.this", "name", "Living Room"),
					resource.TestCheckResourceAttr("omada_access_point.this", "model", "EAP670"),
					// Not configured, so read back as they were.
					resource.TestCheckResourceAttr("omada_access_point.this", "led_setting", "2"),
					resource.TestCheckResourceAttr("omada_access_point.this", "radio_5g_channel_width", "6"),
					resource.TestCheckResourceAttr("omada_access_point.this", "radio_5g_tx_power", "28"),
					// channel is read-only and reflects the device.
					resource.TestCheckResourceAttr("omada_access_point.this", "radio_5g_channel", "0"),
					noRadioWrites,
					checkUntouched,
				),
			},
			{
				// Restating a radio's CURRENT values must also write nothing:
				// equality is checked against the device, not against the plan.
				Config: testProviderConfig(srv.URL) + `
resource "omada_access_point" "this" {
  mac                    = "10:5a:95:89:c3:64"
  name                   = "Living Room"
  radio_5g_channel_width = "6"
  radio_5g_tx_power      = 28
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_access_point.this", "radio_5g_tx_power", "28"),
					noRadioWrites,
				),
			},
			{
				// A real change writes exactly one radio, and only that one.
				Config: testProviderConfig(srv.URL) + `
resource "omada_access_point" "this" {
  mac               = "10:5a:95:89:c3:64"
  name              = "Living Room"
  radio_5g_tx_power = 20
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_access_point.this", "radio_5g_tx_power", "20"),
					func(*terraform.State) error {
						if got := radioWrites("radioSetting5g"); got != 1 {
							return fmt.Errorf("expected exactly one 5GHz radio write, got %d", got)
						}
						if got := radioWrites("radioSetting2g"); got != 0 {
							return fmt.Errorf("the 2.4GHz radio was restarted for a 5GHz change: %d write(s)", got)
						}
						return nil
					},
					checkUntouched,
				),
			},
			{
				ResourceName:      "omada_access_point.this",
				ImportState:       true,
				ImportStateId:     "10-5A-95-89-C3-64",
				ImportStateVerify: true,
				// State keeps the config's colon spelling; import reads the
				// controller's dashes. Same textual-comparison limitation as
				// omada_gateway and omada_switch_port.
				ImportStateVerifyIgnore: []string{"mac"},
			},
		},
	})
}

// TestAccAccessPointResourceSingleBandRadio covers a device that reports no
// 5GHz radio at all (single-band hardware, or that band not adopted) —
// distinct from the dual-band mock AP's 0-valued fields above.
//
// Configuring radio_5g_* against it must error, not silently drop the value:
// changed()/radioBody() previously returned nil (the same as "nothing to
// send") whenever cur was nil, so the configured value never reached the
// PATCH, and the subsequent refresh then wrote state back with it
// false/empty — contradicting the config with no error at all.
func TestAccAccessPointResourceSingleBandRadio(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// No radio_5g_* configured: must not error just because the
				// device has no 5GHz radio.
				Config: testProviderConfig(srv.URL) + `
resource "omada_access_point" "single" {
  mac  = "aa:bb:cc:11:22:33"
  name = "Yard"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_access_point.single", "model", "EAP225-Outdoor"),
					resource.TestCheckResourceAttr("omada_access_point.single", "radio_5g_enable", "false"),
				),
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_access_point" "single" {
  mac               = "aa:bb:cc:11:22:33"
  name              = "Yard"
  radio_5g_tx_power = 20
}`,
				ExpectError: regexp.MustCompile(`device reports no 5g radio at all`),
			},
		},
	})
}
