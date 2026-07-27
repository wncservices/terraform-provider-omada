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

// TestAccIoTBeaconResource drives create → import → update of an iBeacon
// profile.
//
// This test carries more weight than most in this provider: the development
// site has no BLE-capable access point, so the controller refuses every create
// and the mock is the only place the create and delete paths run at all. It
// therefore emulates the controller's IoT device inventory rather than
// accepting any MAC.
func TestAccIoTBeaconResource(t *testing.T) {
	srv := newMockController(t)

	checkUnmodelledPreserved := func(*terraform.State) error {
		doc := rawStore(t, srv.URL, "iotBeacons")["beacon-default"]
		if got, _ := doc["unmodelledKey"].(string); got != "keep-me" {
			return fmt.Errorf("unmodelled key was dropped: %v", doc)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Lower-case colon spelling, to prove the MAC is normalised to
				// the controller's dashed upper-case form before it is sent —
				// the inventory check in the mock is exact, so an unnormalised
				// MAC would be rejected with -33284.
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_beacon" "lobby" {
  name        = "lobby"
  uuid        = "0123456789abcdef0123456789abcdef"
  device_macs = ["60:83:e7:4b:1b:40"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_iot_beacon.lobby", "id"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "name", "lobby"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "site_id", "site-1"),
					// Staged off by default.
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "enable", "false"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "major", "0000"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "measure_power", "-65"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "adv_interval", "500"),
					// State keeps the spelling from the config — semantic equality
					// means it is not rewritten. That the *wire* value was
					// normalised is proved by the mock having accepted it: its
					// inventory check matches dashed upper-case exactly, so an
					// unnormalised MAC would have been refused with -33284.
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "device_macs.0", "60:83:e7:4b:1b:40"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "bound_device_num", "1"),
				),
			},
			{
				ResourceName:      "omada_iot_beacon.lobby",
				ImportState:       true,
				ImportStateVerify: true,
				// Import reads the controller's dashed spelling while the prior
				// state holds the config's colons. The provider treats those as
				// the same value, but ImportStateVerify compares state strings
				// textually and cannot know that — the same limitation as
				// omada_switch_port.switch_mac.
				ImportStateVerifyIgnore: []string{"device_macs.0"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_beacon" "lobby" {
  name         = "lobby (live)"
  uuid         = "0123456789abcdef0123456789abcdef"
  major        = "0001"
  minor        = "0002"
  enable       = true
  adv_interval = 1000
  device_macs  = ["60-83-E7-4B-1B-40", "DC-62-79-97-7B-82"]
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "name", "lobby (live)"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "enable", "true"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "major", "0001"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "minor", "0002"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "adv_interval", "1000"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "device_macs.#", "2"),
					resource.TestCheckResourceAttr("omada_iot_beacon.lobby", "bound_device_num", "2"),
					// The pre-existing profile is untouched throughout.
					checkUnmodelledPreserved,
				),
			},
		},
	})
}

// TestAccIoTBeaconRejectsEmptyDeviceMACs covers the check the provider makes
// before the controller does, so the message names the attribute.
func TestAccIoTBeaconRejectsEmptyDeviceMACs(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_beacon" "lobby" {
  name        = "lobby"
  uuid        = "0123456789abcdef0123456789abcdef"
  device_macs = []
}`,
				ExpectError: regexp.MustCompile(`device_macs must name`),
			},
		},
	})
}

// TestAccIoTBeaconRejectsNonIoTDevice pins the failure a practitioner meets on
// access points without a BLE radio — which is every AP on the development
// site, and the reason create could not be verified live.
func TestAccIoTBeaconRejectsNonIoTDevice(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_iot_beacon" "lobby" {
  name        = "lobby"
  uuid        = "0123456789abcdef0123456789abcdef"
  device_macs = ["8C-86-DD-10-50-CA"]
}`,
				ExpectError: regexp.MustCompile(`not in the current site`),
			},
		},
	})
}
