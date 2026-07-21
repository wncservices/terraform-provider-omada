// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories wires our provider for the acceptance harness.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"omada": providerserver.NewProtocol6WithError(New("test")()),
}

// TestProviderSchema is a pure unit test (always runs) that the provider's
// schema and metadata build without diagnostics.
func TestProviderSchema(t *testing.T) {
	p := New("test")()

	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("provider schema has errors: %v", schemaResp.Diagnostics)
	}
	for _, attr := range []string{"url", "username", "password", "skip_tls_verify", "site"} {
		if _, ok := schemaResp.Schema.Attributes[attr]; !ok {
			t.Errorf("provider schema missing attribute %q", attr)
		}
	}

	metaResp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), provider.MetadataRequest{}, metaResp)
	if metaResp.TypeName != "omada" {
		t.Errorf("provider type name = %q, want omada", metaResp.TypeName)
	}
}

// newMockController returns an httptest server that emulates the Omada v6
// info + login handshake plus the sites and networks list endpoints, so
// acceptance tests run in CI without a real controller.
func newMockController(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/info", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, 0, "", map[string]any{
			"omadacId": "abc123", "controllerVer": "6.0.0", "apiVer": "3", "type": 1,
		})
	})

	mux.HandleFunc("/abc123/api/v2/login", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, 0, "", map[string]any{"token": "tok-xyz"})
	})

	requireToken := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Csrf-Token") != "tok-xyz" {
			writeEnvelope(w, -1400, "invalid csrf token", nil)
			return false
		}
		return true
	}

	mux.HandleFunc("/abc123/api/v2/sites", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		writeEnvelope(w, 0, "", map[string]any{
			"totalRows": 1, "currentPage": 1, "currentSize": 100,
			"data": []map[string]any{{"id": "site-1", "name": "Default"}},
		})
	})

	mux.HandleFunc("/abc123/api/v2/sites/site-1/setting/lan/networks", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		writeEnvelope(w, 0, "", map[string]any{
			"totalRows": 1, "currentPage": 1, "currentSize": 100,
			"data": []map[string]any{{
				"id": "net-1", "name": "IoT", "purpose": "interface",
				"vlan": 30, "gatewaySubnet": "192.168.30.1/24", "dhcpGuardEnable": true,
			}},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeEnvelope(w http.ResponseWriter, code int, msg string, result any) {
	var raw json.RawMessage
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"errorCode": code, "msg": msg, "result": raw})
}

// testProviderConfig renders a provider block pointed at the mock controller.
func testProviderConfig(url string) string {
	return fmt.Sprintf(`
provider "omada" {
  url             = %q
  username        = "admin"
  password        = "secret"
  skip_tls_verify = true
}
`, url)
}
