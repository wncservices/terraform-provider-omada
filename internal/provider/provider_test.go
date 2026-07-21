// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	// Stateful LAN-networks store, seeded with one network for the data-source
	// tests and mutated by the resource acceptance test (create/update/delete).
	var mu sync.Mutex
	nextID := 1
	networks := map[string]map[string]any{
		"net-1": {
			"id": "net-1", "name": "IoT", "purpose": "interface",
			"vlan": 30, "gatewaySubnet": "192.168.30.1/24",
			"dhcpSettings": map[string]any{"enable": true, "ipaddrStart": "192.168.30.2", "ipaddrEnd": "192.168.30.254", "leasetime": 120},
		},
	}
	const netBase = "/abc123/api/v2/sites/site-1/setting/lan/networks"

	// Collection: GET (list) + POST (create).
	mux.HandleFunc(netBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("gen-%d", nextID)
			nextID++
			in["id"] = id
			networks[id] = in
			writeEnvelope(w, 0, "", in)
		default: // GET
			data := make([]map[string]any, 0, len(networks))
			for _, n := range networks {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{
				"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data,
			})
		}
	})

	// Item: PATCH (update) + DELETE.
	mux.HandleFunc(netBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, netBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(networks, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			cur := networks[id]
			if cur == nil {
				cur = map[string]any{}
			}
			for k, v := range in {
				cur[k] = v
			}
			cur["id"] = id
			networks[id] = cur
			writeEnvelope(w, 0, "", cur)
		}
	})

	// Stateful LAN DNS store. Create returns a null result (like the real
	// controller), so the client resolves the new record by name via GET.
	dns := map[string]map[string]any{}
	dnsNext := 1
	const dnsBase = "/abc123/api/v2/sites/site-1/setting/lan/dns"
	mux.HandleFunc(dnsBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("dns-%d", dnsNext)
			dnsNext++
			in["id"] = id
			dns[id] = in
			writeEnvelope(w, 0, "", nil)
		default: // GET
			data := make([]map[string]any, 0, len(dns))
			for _, n := range dns {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(dnsBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, dnsBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(dns, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			cur := dns[id]
			if cur == nil {
				cur = map[string]any{}
			}
			for k, v := range in {
				cur[k] = v
			}
			cur["id"] = id
			dns[id] = cur
			writeEnvelope(w, 0, "", cur)
		}
	})

	// Stateful port-forward store, seeded with one rule so the WAN-port default
	// can be inferred (mirrors a real controller with an existing rule).
	pf := map[string]map[string]any{
		"pf-seed": {"id": "pf-seed", "name": "seed", "status": true, "protocol": 1,
			"externalPort": "80", "forwardIp": "10.10.20.9", "forwardPort": "80",
			"interfaceWanPortId": []any{"wan-1"}, "virtualWanId": []any{}, "dMZ": false},
	}
	pfNext := 1
	const pfBase = "/abc123/api/v2/sites/site-1/setting/transmission/portForwardings"
	mux.HandleFunc(pfBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("pf-%d", pfNext)
			pfNext++
			in["id"] = id
			pf[id] = in
			writeEnvelope(w, 0, "", nil)
		default: // GET
			data := make([]map[string]any, 0, len(pf))
			for _, n := range pf {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"totalRows": len(data), "data": data})
		}
	})
	mux.HandleFunc(pfBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, pfBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(pf, id)
			writeEnvelope(w, 0, "", nil)
		default: // PUT
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			pf[id] = in
			writeEnvelope(w, 0, "", in)
		}
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
