// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccControllerSettingsResource drives create → import → update of the
// controller-scoped settings document.
//
// Note every config here: there is no `site` argument. This is the first
// resource in the provider that takes none, and a regression that reintroduced
// one would fail to parse these.
func TestAccControllerSettingsResource(t *testing.T) {
	srv := newMockController(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_controller_settings" "this" {
  name        = "5398"
  time_zone   = "America/Los_Angeles"

  host_name          = "omada.example.com"
  device_host_enable = true
  device_host        = "10.0.54.10"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_controller_settings.this", "name", "5398"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "time_zone", "America/Los_Angeles"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "host_name", "omada.example.com"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "device_host_enable", "true"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "device_host", "10.0.54.10"),

					// The id is the controller's own, not a site's.
					resource.TestCheckResourceAttr("omada_controller_settings.this", "id", "abc123"),

					// Never set in config, so these come back from the
					// controller rather than being null.
					resource.TestCheckResourceAttr("omada_controller_settings.this", "region", "United States"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "manage_https_port", "8043"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "app_discovery", "true"),
					resource.TestCheckResourceAttr("omada_controller_settings.this", "log_level_type", "AUTO"),

					// The sections nothing in the config touches must be
					// untouched on the controller, including the one carrying
					// SMTP credentials that the provider does not model at all.
					checkControllerSection(srv.URL, "mailServer", "password", "seeded-smtp-password"),
					checkControllerSection(srv.URL, "deviceAccessManagement", "unmodelledKey", "keep-me"),
				),
			},
			{
				ResourceName: "omada_controller_settings.this",
				ImportState:  true,
				// The document is a singleton, so the import id is ignored.
				ImportStateId:     "controller",
				ImportStateVerify: true,
			},
			{
				// Change one field in one section. Everything else, in that
				// section and the others, has to survive.
				Config: testProviderConfig(srv.URL) + `
resource "omada_controller_settings" "this" {
  name        = "5398"
  time_zone   = "America/Los_Angeles"

  host_name          = "omada.example.com"
  device_host_enable = true
  device_host        = "10.0.54.10"

  device_web_control_http = false
  app_discovery           = false
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_controller_settings.this", "app_discovery", "false"),
					// Same section, not in config: read, preserved, sent back.
					resource.TestCheckResourceAttr("omada_controller_settings.this", "device_web_control_https", "true"),
					checkControllerSection(srv.URL, "deviceAccessManagement", "unmodelledKey", "keep-me"),
					checkControllerSection(srv.URL, "mailServer", "password", "seeded-smtp-password"),
				),
			},
		},
	})
}

// checkControllerSection asserts a raw key inside one section of the mock's
// stored document, for keys the provider deliberately does not model.
func checkControllerSection(base, section, key string, want any) resource.TestCheckFunc {
	return func(*terraform.State) error {
		resp, err := http.Get(base + "/debug/controllerSettings")
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		var doc map[string]map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			return err
		}
		if got := doc[section][key]; got != want {
			return fmt.Errorf("controller settings %s.%s = %v, want %v", section, key, got, want)
		}
		return nil
	}
}
