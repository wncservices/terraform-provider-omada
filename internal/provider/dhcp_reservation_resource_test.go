// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDHCPReservationResource drives create → import → update.
//
// Two controller behaviours are the real subject here. The item path is keyed
// on the MAC rather than the id — and the controller answers 0 for a key that
// matched nothing, so a provider keyed on the id would appear to work while
// silently doing nothing. And MACs are normalised, so config written with
// colons must not diff against the controller's dash-separated form.
func TestAccDHCPReservationResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dhcp_reservation" "nas" {
  network_id = "net-1"
  mac        = "00:11:32:be:7f:73"
  ip         = "10.10.20.50"
  name       = "Moria"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_dhcp_reservation.nas", "id"),
					// Written with colons: the practitioner's spelling is kept,
					// while the controller stores the dash form. Neither diffs.
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "mac", "00:11:32:be:7f:73"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "ip", "10.10.20.50"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "name", "Moria"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "enable", "true"),
					// Controller-forced, reported not managed.
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "export_to_ip_mac_binding", "true"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_dhcp_reservation.nas",
				ImportState:       true,
				ImportStateId:     "00-11-32-BE-7F-73",
				ImportStateVerify: true,
				// ImportStateVerify compares raw strings, so it cannot see the
				// semantic equality that macType provides — the imported value
				// is the same MAC in the controller's spelling. The live plan
				// confirms no diff and no replacement.
				ImportStateVerifyIgnore: []string{"mac"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_dhcp_reservation" "nas" {
  network_id = "net-1"
  mac        = "00:11:32:be:7f:73"
  ip         = "10.10.20.51"
  name       = "Moria (moved)"
  enable     = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "ip", "10.10.20.51"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "name", "Moria (moved)"),
					resource.TestCheckResourceAttr("omada_dhcp_reservation.nas", "enable", "false"),
				),
			},
		},
	})
}
