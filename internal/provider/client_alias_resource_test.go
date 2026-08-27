// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// TestAccClientAliasResource drives adopt -> import -> update. The mock rejects
// any PATCH body containing more than name, so the test also proves that client
// runtime and unrelated persistent settings are never round-tripped.
func TestAccClientAliasResource(t *testing.T) {
	srv := newMockController(t)

	checkClient := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			client := rawStore(t, srv.URL, "clients")["00-11-22-33-44-55"]
			if got, _ := client["name"].(string); got != want {
				return fmt.Errorf("client alias = %q, want %q", got, want)
			}
			if _, ok := client["ipSetting"]; !ok {
				return fmt.Errorf("client ipSetting was removed")
			}
			if _, ok := client["rateLimit"]; !ok {
				return fmt.Errorf("client rateLimit was removed")
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkClient("00-11-22-33-44-55"),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00:11:22:33:44:55"
  alias = "Printer"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_client_alias.printer", "id", "00-11-22-33-44-55"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "mac", "00:11:22:33:44:55"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "alias", "Printer"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "site", "Default"),
					resource.TestCheckResourceAttr("omada_client_alias.printer", "site_id", "site-1"),
					checkClient("Printer"),
				),
			},
			{
				ResourceName:            "omada_client_alias.printer",
				ImportState:             true,
				ImportStateId:           "00-11-22-33-44-55",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"mac"},
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00:11:22:33:44:55"
  alias = "Office Printer"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("omada_client_alias.printer", "alias", "Office Printer"),
					checkClient("Office Printer"),
				),
			},
		},
	})
}

func TestClientAliasNotFoundClassification(t *testing.T) {
	t.Parallel()
	if !clientAliasNotFound(fmt.Errorf("wrapped: %w", &omada.APIError{Code: -34326, Msg: "gone"})) {
		t.Fatal("-34326 should be classified as a missing client")
	}
	for _, err := range []error{
		&omada.APIError{Code: -1, Msg: "transient"},
		fmt.Errorf("transport failed"),
	} {
		if clientAliasNotFound(err) {
			t.Fatalf("%v should not be classified as a missing client", err)
		}
	}
}

func TestClientAliasImportRejectsInvalidMAC(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"", "../clients", "Default/not-a-mac", "/00-11-22-33-44-55", "Default/00-11-22-33-44-55/extra"} {
		resp := &frameworkresource.ImportStateResponse{}
		(&clientAliasResource{}).ImportState(context.Background(), frameworkresource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("import id %q was accepted", id)
		}
	}
}

func TestAccClientAliasRejectsInvalidMAC(t *testing.T) {
	srv := newMockController(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "invalid" {
  mac   = "../clients"
  alias = "Invalid"
}`,
			ExpectError: regexp.MustCompile(`Invalid MAC address`),
		}},
	})
}

func TestAccClientAliasRejectsBlankAlias(t *testing.T) {
	srv := newMockController(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "blank" {
  mac   = "00-11-22-33-44-55"
  alias = ""
}`,
				ExpectError: regexp.MustCompile(`must contain at least one non-whitespace character`),
			},
			{
				Config: testProviderConfig(srv.URL) + `
resource "omada_client_alias" "blank" {
  mac   = "00-11-22-33-44-55"
  alias = "  "
}`,
				ExpectError: regexp.MustCompile(`must contain at least one non-whitespace character`),
			},
		},
	})
}

func setMockClientReadError(t *testing.T, base string, code int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/debug/clients?error=%d", base, code), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("setting mock client read error: http %d", resp.StatusCode)
	}
}

func TestAccClientAliasRefreshErrorPreservesState(t *testing.T) {
	srv := newMockController(t)
	config := testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00-11-22-33-44-55"
  alias = "Printer"
}`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:   func() { setMockClientReadError(t, srv.URL, -1) },
				Config:      config,
				ExpectError: regexp.MustCompile(`Unable to refresh client alias`),
			},
			{
				PreConfig: func() { setMockClientReadError(t, srv.URL, 0) },
				Config:    config,
			},
		},
	})
}

func TestAccClientAliasMissingClientPlansReadoption(t *testing.T) {
	srv := newMockController(t)
	config := testProviderConfig(srv.URL) + `
resource "omada_client_alias" "printer" {
  mac   = "00-11-22-33-44-55"
  alias = "Printer"
}`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:          func() { setMockClientReadError(t, srv.URL, -34326) },
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: func() { setMockClientReadError(t, srv.URL, 0) },
				Config:    config,
			},
		},
	})
}
