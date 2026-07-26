// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccServiceTypeResource drives create → import → update of a custom
// service type, and checks the two things that distinguish this endpoint:
// create answers with the id as a bare string rather than an object, and
// `is_builtin` is reported but never sent.
func TestAccServiceTypeResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_service_type" "grafana" {
  name              = "grafana"
  protocol          = 0
  destination_ports = "3000-3000"
  description       = "Grafana UI"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("omada_service_type.grafana", "id"),
					resource.TestCheckResourceAttr("omada_service_type.grafana", "name", "grafana"),
					resource.TestCheckResourceAttr("omada_service_type.grafana", "destination_ports", "3000-3000"),
					// Defaulted, not stated in config.
					resource.TestCheckResourceAttr("omada_service_type.grafana", "source_ports", "0-65535"),
					// A custom type is never a built-in.
					resource.TestCheckResourceAttr("omada_service_type.grafana", "is_builtin", "false"),
					resource.TestCheckResourceAttr("omada_service_type.grafana", "site_id", "site-1"),
				),
			},
			{
				ResourceName:      "omada_service_type.grafana",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_service_type" "grafana" {
  name              = "grafana-alt"
  protocol          = 0
  source_ports      = "1024-65535"
  destination_ports = "3000-3001"
  description       = "Grafana UI (alt)"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_service_type.grafana", "name", "grafana-alt"),
					resource.TestCheckResourceAttr("omada_service_type.grafana", "source_ports", "1024-65535"),
					resource.TestCheckResourceAttr("omada_service_type.grafana", "destination_ports", "3000-3001"),
				),
			},
		},
	})
}
