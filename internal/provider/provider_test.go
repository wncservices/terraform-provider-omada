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
			"vlan": 30, "vlanType": 0, "application": 0,
			"gatewaySubnet": "192.168.30.1/24",
			// per-VLAN switching/security toggles
			"isolation": true, "allLan": false, "portal": false, "rateLimit": false,
			"qosQueueEnable": false, "accessControlRule": true, "arpDetectionEnable": true,
			"igmpSnoopEnable": false, "fastLeaveEnable": false, "mldSnoopEnable": false,
			"dhcpL2RelayEnable":    false,
			"dhcpGuard":            map[string]any{"enable": false},
			"dhcpv6Guard":          map[string]any{"enable": false},
			"lanNetworkIpv6Config": map[string]any{"enable": 0},
			// derived keys the provider must preserve on update
			"ipRangePool": []map[string]any{{"ipaddrStart": "192.168.30.2", "ipaddrEnd": "192.168.30.254"}},
			"totalIpNum":  253,
			"dhcpSettings": map[string]any{
				"enable": true, "ipaddrStart": "192.168.30.2", "ipaddrEnd": "192.168.30.254",
				"leasetime": 120, "dhcpns": "auto",
				"options": []map[string]any{{"code": 138, "type": 1, "value": "10.10.20.50"}},
			},
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

	// Stateful IP-group store. Item operations use /groups/{type}/{groupId}.
	groups := map[string]map[string]any{}
	grpNext := 1
	const grpBase = "/abc123/api/v2/sites/site-1/setting/profiles/groups"
	mux.HandleFunc(grpBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("grp-%d", grpNext)
			grpNext++
			in["groupId"] = id
			if in["type"] == nil {
				in["type"] = 0
			}
			groups[id] = in
			writeEnvelope(w, 0, "", nil)
		default: // GET
			data := make([]map[string]any, 0, len(groups))
			for _, n := range groups {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(grpBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, grpBase+"/"), "/")
		id := parts[len(parts)-1]
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(groups, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			cur := groups[id]
			if cur == nil {
				cur = map[string]any{}
			}
			for k, v := range in {
				cur[k] = v
			}
			cur["groupId"] = id
			groups[id] = cur
			writeEnvelope(w, 0, "", cur)
		}
	})

	// Stateful firewall-ACL store (POST create, GET ?type=N, PUT /{id}, DELETE).
	// Seeded with one gateway rule so the omada_firewall_acls data source has
	// something to list; the resource tests only assert on rules they create.
	acls := map[string]map[string]any{
		"acl-seed": {"id": "acl-seed", "type": 0 /* gateway */, "name": "seed",
			"status": true, "policy": 1},
	}
	aclNext := 1
	const aclBase = "/abc123/api/v2/sites/site-1/setting/firewall/acls"
	mux.HandleFunc(aclBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("acl-%d", aclNext)
			aclNext++
			in["id"] = id
			acls[id] = in
			writeEnvelope(w, 0, "", nil)
		default: // GET — filter by ?type=N like the real controller
			want := r.URL.Query().Get("type")
			data := make([]map[string]any, 0, len(acls))
			for _, n := range acls {
				if want != "" && fmt.Sprintf("%v", n["type"]) != want {
					continue
				}
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"totalRows": len(data), "data": data})
		}
	})
	mux.HandleFunc(aclBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, aclBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(acls, id)
			writeEnvelope(w, 0, "", nil)
		default: // PUT
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			acls[id] = in
			writeEnvelope(w, 0, "", in)
		}
	})

	// Stateful WLAN-group store (POST create, GET list, PATCH /{id}, DELETE).
	wlans := map[string]map[string]any{}
	wlanNext := 1
	const wlanBase = "/abc123/api/v2/sites/site-1/setting/wlans"
	mux.HandleFunc(wlanBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("wlan-%d", wlanNext)
			wlanNext++
			in["id"] = id
			in["primary"] = false
			wlans[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(wlans))
			for _, n := range wlans {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(wlanBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, wlanBase+"/")
		if strings.Contains(id, "/") { // e.g. .../wlans/{id}/ssids — not a group item
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(wlans, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			cur := wlans[id]
			if cur == nil {
				cur = map[string]any{}
			}
			for k, v := range in {
				cur[k] = v
			}
			cur["id"] = id
			wlans[id] = cur
			writeEnvelope(w, 0, "", cur)
		}
	})

	// Stateful mDNS store (POST create, GET list, PUT /{id}, DELETE).
	mdns := map[string]map[string]any{}
	mdnsNext := 1
	const mdnsBase = "/abc123/api/v2/sites/site-1/setting/service/mdns"
	mux.HandleFunc(mdnsBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("mdns-%d", mdnsNext)
			mdnsNext++
			in["id"] = id
			mdns[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(mdns))
			for _, n := range mdns {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"totalRows": len(data), "data": data})
		}
	})
	mux.HandleFunc(mdnsBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, mdnsBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(mdns, id)
			writeEnvelope(w, 0, "", nil)
		default: // PUT
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			mdns[id] = in
			writeEnvelope(w, 0, "", in)
		}
	})

	// Stateful port-profile store (POST create, GET list, PATCH /{id}, DELETE).
	profs := map[string]map[string]any{}
	profNext := 1
	const profBase = "/abc123/api/v2/sites/site-1/setting/lan/profiles"
	mux.HandleFunc(profBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("prof-%d", profNext)
			profNext++
			in["id"] = id
			// Simulate controller-owned keys the provider must not clobber.
			in["prohibitModify"] = false
			in["flag"] = 2
			if stp, ok := in["spanningTreeSetting"].(map[string]any); ok {
				stp["instances"] = []any{}
			}
			profs[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(profs))
			for _, n := range profs {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(profBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, profBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(profs, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			profs[id] = in
			writeEnvelope(w, 0, "", in)
		}
	})

	// Stateful SSID store (nested under /wlans/{gid}/ssids — Go 1.22 wildcards).
	ssids := map[string]map[string]any{}
	ssidNext := 1
	mux.HandleFunc("/abc123/api/v2/sites/site-1/setting/wlans/{gid}/ssids", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("ssid-%d", ssidNext)
			ssidNext++
			in["id"] = id
			ssids[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(ssids))
			for _, n := range ssids {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc("/abc123/api/v2/sites/site-1/setting/wlans/{gid}/ssids/{sid}", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := r.PathValue("sid")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(ssids, id)
			writeEnvelope(w, 0, "", nil)
		default: // PATCH
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			ssids[id] = in
			writeEnvelope(w, 0, "", in)
		}
	})

	// Stateful VPN store (POST create, GET list, PUT /{id}, DELETE).
	vpns := map[string]map[string]any{}
	vpnNext := 1
	const vpnBase = "/abc123/api/v2/sites/site-1/setting/vpns"
	mux.HandleFunc(vpnBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("vpn-%d", vpnNext)
			vpnNext++
			in["id"] = id
			vpns[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(vpns))
			for _, n := range vpns {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(vpnBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, vpnBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(vpns, id)
			writeEnvelope(w, 0, "", nil)
		default: // PUT
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			vpns[id] = in
			writeEnvelope(w, 0, "", in)
		}
	})

	// Stateful static-route store (POST create, GET list, PUT /{id}, DELETE).
	routes := map[string]map[string]any{}
	routeNext := 1
	const routeBase = "/abc123/api/v2/sites/site-1/setting/transmission/staticRoutings"
	mux.HandleFunc(routeBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("route-%d", routeNext)
			routeNext++
			in["id"] = id
			routes[id] = in
			writeEnvelope(w, 0, "", nil)
		default:
			data := make([]map[string]any, 0, len(routes))
			for _, n := range routes {
				data = append(data, n)
			}
			writeEnvelope(w, 0, "", map[string]any{"totalRows": len(data), "data": data})
		}
	})
	mux.HandleFunc(routeBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, routeBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(routes, id)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPut:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			routes[id] = in
			writeEnvelope(w, 0, "", in)
		default:
			// The real controller rejects anything but PUT here; mirror that so a
			// regression back to PATCH fails the test instead of silently passing.
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})

	// WAN settings (read-only): the real payload mixes config with a large set of
	// read-only support* capability flags, which the data source must ignore.
	mux.HandleFunc("/abc123/api/v2/sites/site-1/setting/wan/networks", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		writeEnvelope(w, 0, "", map[string]any{
			"supportPppoe": true, "supportIpv6": true, "portNum": 2,
			"wanPortSettings": []map[string]any{{
				"portUuid": "wan-1", "portName": "WAN/LAN1",
				"wanPortIpv4Setting": map[string]any{
					"proto": "dhcp", "protoType": 0, "vlanId": 0, "qosTagEnable": false,
					"ipv4Dhcp": map[string]any{"unicast": false, "mtu": 1500},
				},
				"wanPortIpv6Setting": map[string]any{"enable": 0},
				"wanPortMacSetting":  map[string]any{"method": "recover", "mac": "AA-BB-CC-DD-EE-FF"},
			}},
		})
	})

	// Stateful captive-portal store. Like the real controller: the list result is
	// a bare JSON array, create returns a null result, update is PATCH with a full
	// read-modify-write payload. `simplePassword` is stored but never returned in
	// the list, mirroring the write-only password.
	portals := map[string]map[string]any{}
	portalNext := 1
	const portalBase = "/abc123/api/v2/sites/site-1/setting/portals"
	// portalPublic strips the write-only password from a stored portal.
	portalPublic := func(p map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range p {
			if k == "simplePassword" {
				continue
			}
			out[k] = v
		}
		return out
	}
	mux.HandleFunc(portalBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("portal-%d", portalNext)
			portalNext++
			in["id"] = id
			// Controller-owned defaults the provider must preserve on update.
			if _, ok := in["portalCustomize"]; !ok {
				in["portalCustomize"] = map[string]any{
					"defaultLanguage": 1, "copyrightEnable": false, "buttonText": "Log In",
				}
			}
			portals[id] = in
			writeEnvelope(w, 0, "", nil)
		default: // GET — bare array
			data := make([]map[string]any, 0, len(portals))
			for _, p := range portals {
				data = append(data, portalPublic(p))
			}
			writeEnvelope(w, 0, "", data)
		}
	})
	mux.HandleFunc(portalBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, portalBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(portals, id)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPatch:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			cur := portals[id]
			if cur == nil {
				cur = map[string]any{}
			}
			// The real controller rejects a patch whose portalCustomize lost its
			// required keys — assert the provider sent them back.
			if pc, ok := in["portalCustomize"].(map[string]any); ok {
				if _, has := pc["defaultLanguage"]; !has {
					writeEnvelope(w, -1001, "portalCustomize parameter [defaultLanguage] should not be null", nil)
					return
				}
			}
			for k, v := range in {
				cur[k] = v
			}
			cur["id"] = id
			portals[id] = cur
			writeEnvelope(w, 0, "", nil)
		default:
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})
	// Unauthenticated debug endpoint so tests can assert the stored password.
	mux.HandleFunc("/debug/portals", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(portals)
	})

	// Devices list — result is a bare JSON array (no pagination envelope), like
	// the real controller. Seeded with one switch and one AP.
	mux.HandleFunc("/abc123/api/v2/sites/site-1/devices", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		writeEnvelope(w, 0, "", []map[string]any{
			{"name": "SW-1", "type": "switch", "model": "ES205GP", "mac": "8C-86-DD-10-50-CA",
				"sn": "225", "ip": "10.10.99.2", "status": 14, "statusCategory": 1,
				"firmwareVersion": "1.0.6 Build 20260329", "version": "1.0.6",
				"needUpgrade": false, "uptimeLong": 5656402, "clientNum": 0, "hwVersion": "1.0"},
			{"name": "AP-1", "type": "ap", "model": "EAP610", "mac": "60-83-E7-4B-1B-40",
				"sn": "226", "ip": "10.10.99.4", "status": 14, "statusCategory": 1,
				"firmwareVersion": "1.2.3", "version": "1.2.3",
				"needUpgrade": true, "uptimeLong": 1000, "clientNum": 7, "hwVersion": "1.0"},
		})
	})

	// Site-settings singleton (GET /setting object, PATCH merges top-level groups).
	// deviceAccount is included deliberately: the provider must never send it,
	// so it should survive every update untouched.
	siteSettings := map[string]any{
		"led":                      map[string]any{"enable": true},
		"lldp":                     map[string]any{"enable": true},
		"advancedFeature":          map[string]any{"enable": true},
		"autoUpgrade":              map[string]any{"enable": false},
		"channelLimit":             map[string]any{"enable": false},
		"rememberDevice":           map[string]any{"enable": true},
		"airtimeFairness":          map[string]any{"enable2g": false, "enable5g": false, "enable6g": false},
		"alert":                    map[string]any{"enable": false, "delayEnable": true, "delay": 60},
		"bandSteering":             map[string]any{"enable": false, "connectionThreshold": 30, "differenceThreshold": 4, "maxFailures": 5},
		"bandSteeringForMultiBand": map[string]any{"mode": 1},
		"mesh":                     map[string]any{"meshEnable": true, "autoFailoverEnable": true, "defGatewayEnable": true, "fullSector": true},
		"remoteLog":                map[string]any{"enable": false, "port": 514, "moreClientLog": false},
		"speedTest":                map[string]any{"enable": false, "interval": 120},
		"roaming": map[string]any{
			"fastRoamingEnable": true, "aiRoamingEnable": false, "dualBand11kReportEnable": true,
			"forceDisassociationEnable": false, "nonStickRoamingEnable": false, "nonPingPongRoamingEnable": false,
		},
		"beaconControl": map[string]any{
			"beaconIntvMode2g": 0, "dtimPeriod2g": 1, "rtsThreshold2g": 2347, "fragmentationThreshold2g": 2346,
			"beaconIntvMode5g": 0, "dtimPeriod5g": 1, "rtsThreshold5g": 2347, "fragmentationThreshold5g": 2346,
			"beaconInterval6g": 100, "beaconIntvMode6g": 0, "dtimPeriod6g": 1, "rtsThreshold6g": 2347,
			"fragmentationThreshold6g": 2346,
		},
		// #nosec G101 -- test fixture, not a credential: it exists purely to assert
		// the provider never sends deviceAccount in its patch.
		"deviceAccount": map[string]any{"username": "device-admin", "password": "must-not-be-touched"},
	}
	mux.HandleFunc("/abc123/api/v2/sites/site-1/setting", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPatch {
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			for k, v := range in {
				siteSettings[k] = v
			}
		}
		writeEnvelope(w, 0, "", siteSettings)
	})

	// Flat singleton settings documents (ALG, attack defense). Three real
	// controller behaviours are emulated here because the provider depends on
	// each of them:
	//
	//   - PATCH is rejected with -1600; these endpoints require PUT.
	//   - reads carry read-only metadata (`resource`, `support*`, `exist*`) that
	//     the provider must strip before writing back — sending any of it is a
	//     hard error here, so a regression fails the test rather than silently
	//     shipping junk to a real gateway.
	//   - `unmodelledKey` stands in for controller keys the provider does not
	//     model; it must survive an update (read-modify-write).
	// Update verb per endpoint — the controller is not consistent, and the
	// provider records the confirmed verb per SettingDoc. The mock enforces it:
	// the wrong verb answers -1600 exactly as the real controller does.
	singletonVerb := map[string]string{
		"transmission/alg":       http.MethodPut,
		"firewall/attackdefense": http.MethodPut,
		"ssh":                    http.MethodPut,
		"dot1x":                  http.MethodPatch,
		"ips":                    http.MethodPatch,
	}
	singletons := map[string]map[string]any{
		"ssh": {
			"sshEnable": false, "sshServerPort": float64(22), "layer3Access": false,
			"unmodelledKey": "keep-me",
		},
		"dot1x": {
			"enable": false, "authMode": float64(1), "authType": float64(1),
			"macFormat": float64(0), "vlanAssign": false,
			"unmodelledKey": "keep-me",
		},
		// IPS. The *Categories lists are controller-owned reference data: the
		// provider must report them but never send them, so this handler
		// rejects a write that includes one.
		"ips": {
			"enable": true, "ipsMode": float64(1), "geoEnable": true, "dpLevel": float64(3),
			"customCategories": []any{float64(1), float64(2)},
			"lowCategories":    []any{float64(2), float64(3)},
			"mediumCategories": []any{float64(1), float64(2), float64(3)},
			"highCategories":   []any{float64(1), float64(2), float64(3), float64(4)},
			"allCategories":    []any{float64(1), float64(2), float64(3), float64(4)},
			"unmodelledKey":    "keep-me",
		},
		"transmission/alg": {
			"ftp": true, "ftpPorts": []any{float64(21)},
			"h323": true, "pptp": true, "ipSec": true,
			"sip": false, "sipTcp": true, "sipUdp": true,
			"sipPorts":           []any{float64(5060), float64(5061)},
			"sipDirectSignaling": true, "sipDirectMedia": false,
			"sipTimeout": false, "sipSignalingTimeout": float64(3600), "sipMediaTimeout": float64(180),
			"unmodelledKey": "keep-me",
		},
		"firewall/attackdefense": {
			"tcpConnEnable": false, "udpConnEnable": false, "icmpConnEnable": false,
			"tcpSrcEnable": false, "udpSrcEnable": false, "icmpSrcEnable": false,
			"tcpNoflagEnable": true, "tcpScanReject": true,
			"tcpWinnukeEnable": true, "tcpFinSynEnable": true, "tcpFinNoackEnable": true,
			"pingDeathEnable": true, "pingLargeEnable": false, "pingWanEnable": true,
			"ipOptionEnable": true, "ipoptSecureEnable": true,
			"ipoptLooseRouteEnable": true, "ipoptStrictRouteEnable": true,
			"ipoptRecordRouteEnable": true, "ipoptStreamEnable": true,
			"ipoptTimestampEnable": true, "ipoptNoopEnable": true,
			"unmodelledKey": "keep-me",
		},
	}
	// Read-only metadata the controller adds to every read of these documents.
	singletonMeta := map[string]any{
		"resource": float64(0), "supportTcpScanReject": true, "existTcpScanReject": true,
	}
	for suffix, doc := range singletons {
		wantVerb := singletonVerb[suffix]
		mux.HandleFunc("/abc123/api/v2/sites/site-1/setting/"+suffix, func(w http.ResponseWriter, r *http.Request) {
			if !requireToken(w, r) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			switch {
			case r.Method != http.MethodGet && r.Method != wantVerb:
				writeEnvelope(w, -1600, "Unsupported request path.", nil)
				return
			case r.Method == wantVerb:
				var in map[string]any
				_ = json.NewDecoder(r.Body).Decode(&in)
				for k := range in {
					if _, isMeta := singletonMeta[k]; isMeta {
						writeEnvelope(w, -1001, "read-only key "+k+" must not be sent", nil)
						return
					}
					switch k {
					case "lowCategories", "mediumCategories", "highCategories", "allCategories":
						writeEnvelope(w, -1001, "controller-owned key "+k+" must not be sent", nil)
						return
					}
				}
				for k, v := range in {
					doc[k] = v
				}
			}
			out := map[string]any{}
			for k, v := range doc {
				out[k] = v
			}
			for k, v := range singletonMeta {
				out[k] = v
			}
			writeEnvelope(w, 0, "", out)
		})
	}
	// Time-range profiles. Two controller quirks are emulated because the client
	// depends on both: the list envelope carries `data` but **no** `totalRows`
	// (the endpoint does not paginate), and create answers with the new id under
	// `profileId` rather than echoing the object.
	timeRanges := map[string]map[string]any{}
	trNext := 1
	const trBase = "/abc123/api/v2/sites/site-1/setting/profiles/timeranges"
	mux.HandleFunc(trBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("tr-%d", trNext)
			trNext++
			in["id"] = id
			// The controller stamps a ruleId onto each slot; the provider must
			// tolerate reading it back without diffing on it.
			if slots, ok := in["timeList"].([]any); ok {
				for i, sl := range slots {
					if m, ok := sl.(map[string]any); ok {
						m["ruleId"] = float64(1000 + i)
					}
				}
			}
			timeRanges[id] = in
			writeEnvelope(w, 0, "", map[string]any{"profileId": id})
		default: // GET
			data := make([]map[string]any, 0, len(timeRanges))
			for _, t := range timeRanges {
				data = append(data, t)
			}
			writeEnvelope(w, 0, "", map[string]any{"data": data})
		}
	})
	mux.HandleFunc(trBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, trBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(timeRanges, id)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPatch:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			timeRanges[id] = in
			writeEnvelope(w, 0, "", in)
		default:
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})

	// Disable-NAT rules. The asymmetric paths are the point of this handler:
	// the collection is plural (disable-nats) while create and the item path
	// are singular (disable-nat, disable-nat/{id}), and update is PUT — PATCH
	// is rejected, exactly as the controller does.
	disableNats := map[string]map[string]any{}
	dnNext := 1
	const dnList = "/abc123/api/v2/sites/site-1/setting/wired-networks/disable-nats"
	const dnItem = "/abc123/api/v2/sites/site-1/setting/wired-networks/disable-nat"
	mux.HandleFunc(dnList, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		data := make([]map[string]any, 0, len(disableNats))
		for _, d := range disableNats {
			data = append(data, d)
		}
		writeEnvelope(w, 0, "", map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data,
		})
	})
	mux.HandleFunc(dnItem, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost {
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		iface, _ := in["interface"].(string)
		// One rule per WAN port, like the real controller.
		for _, d := range disableNats {
			if cur, _ := d["interface"].(string); cur == iface {
				writeEnvelope(w, -34247, "Only one Disable NAT rule is allowed for one WAN port.", nil)
				return
			}
		}
		id := fmt.Sprintf("dn-%d", dnNext)
		dnNext++
		in["id"] = id
		disableNats[id] = in
		// The controller does not echo the object; the client resolves by name.
		writeEnvelope(w, 0, "", nil)
	})
	mux.HandleFunc(dnItem+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, dnItem+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(disableNats, id)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPut:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = id
			disableNats[id] = in
			writeEnvelope(w, 0, "", in)
		default: // PATCH and anything else
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})

	// DHCP reservations. The trap this reproduces is that the item path is
	// keyed on the **MAC**, not the id, and the controller answers 0 for a key
	// that matched nothing — so a provider keyed on the id would look like it
	// worked while doing nothing at all.
	reservations := map[string]map[string]any{} // by MAC
	resNext := 1
	const dhcpBase = "/abc123/api/v2/sites/site-1/setting/service/dhcp"
	mux.HandleFunc(dhcpBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			mac, _ := in["mac"].(string)
			in["id"] = fmt.Sprintf("res-%d", resNext)
			resNext++
			in["netName"] = "SERVICE"
			// Forced on by the controller no matter what was sent.
			in["exportToIpMacBinding"] = true
			reservations[mac] = in
			writeEnvelope(w, 0, "", in["id"])
		default: // GET
			data := make([]map[string]any, 0, len(reservations))
			for _, d := range reservations {
				data = append(data, d)
			}
			writeEnvelope(w, 0, "", map[string]any{
				"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data,
			})
		}
	})
	mux.HandleFunc(dhcpBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		key := strings.TrimPrefix(r.URL.Path, dhcpBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			// Note: success regardless of whether the key matched, exactly
			// like the controller.
			delete(reservations, key)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPut:
			cur, ok := reservations[key]
			if !ok {
				writeEnvelope(w, -1001, "DHCP Reservation is not exist, please check path param.", nil)
				return
			}
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["id"] = cur["id"]
			in["netName"] = "SERVICE"
			in["exportToIpMacBinding"] = true
			delete(reservations, key)
			mac, _ := in["mac"].(string)
			reservations[mac] = in
			writeEnvelope(w, 0, "", in)
		default: // PATCH
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})

	// RADIUS profiles. Like /setting/portals this is a BARE ARRAY, not a
	// paginated envelope. The secret at authServer[].radiusPwd is stored here
	// so the test can assert it reaches the controller, survives an update
	// that does not re-supply it, and never reaches Terraform state.
	radiusProfiles := map[string]map[string]any{}
	radNext := 1
	const radBase = "/abc123/api/v2/sites/site-1/setting/radiusProfiles"
	mux.HandleFunc(radBase, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			id := fmt.Sprintf("rad-%d", radNext)
			radNext++
			in["radiusProfileId"] = id
			in["builtInServer"] = false
			radiusProfiles[id] = in
			writeEnvelope(w, 0, "", map[string]any{"radiusProfileId": id})
		default: // GET — bare array
			data := make([]map[string]any, 0, len(radiusProfiles))
			for _, p := range radiusProfiles {
				data = append(data, p)
			}
			writeEnvelope(w, 0, "", data)
		}
	})
	mux.HandleFunc(radBase+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, radBase+"/")
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodDelete:
			delete(radiusProfiles, id)
			writeEnvelope(w, 0, "", nil)
		case http.MethodPatch:
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			in["radiusProfileId"] = id
			radiusProfiles[id] = in
			writeEnvelope(w, 0, "", in)
		default:
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
		}
	})
	mux.HandleFunc("/debug/radius", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(radiusProfiles)
	})

	// IPS whitelist. The asymmetric paths are the point: the list is served
	// from a /grid/ view that only answers GET, while create and delete live
	// one level up at /setting/ips/whitelist.
	ipsWhitelist := map[string]map[string]any{}
	ipsNext := 1
	const ipsList = "/abc123/api/v2/sites/site-1/setting/ips/grid/whitelist"
	const ipsItem = "/abc123/api/v2/sites/site-1/setting/ips/whitelist"
	mux.HandleFunc(ipsList, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		data := make([]map[string]any, 0, len(ipsWhitelist))
		for _, e := range ipsWhitelist {
			data = append(data, e)
		}
		writeEnvelope(w, 0, "", map[string]any{
			"totalRows": len(data), "currentPage": 1, "currentSize": 100, "data": data,
		})
	})
	mux.HandleFunc(ipsItem, func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		id := fmt.Sprintf("ipsw-%d", ipsNext)
		ipsNext++
		in["id"] = id
		ipsWhitelist[id] = in
		// Null result, like the controller: the client resolves by matching.
		writeEnvelope(w, 0, "", nil)
	})
	mux.HandleFunc(ipsItem+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireToken(w, r) {
			return
		}
		id := strings.TrimPrefix(r.URL.Path, ipsItem+"/")
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodDelete {
			writeEnvelope(w, -1600, "Unsupported request path.", nil)
			return
		}
		delete(ipsWhitelist, id)
		writeEnvelope(w, 0, "", nil)
	})

	// Notification documents. PATCH-only with the whole document, and the
	// point of the handler is the keyed entry lists: the provider must patch
	// individual entries by key and leave every other entry — and the
	// controller-owned descriptive fields — untouched.
	notifications := map[string]any{
		"alertEmailSetting": map[string]any{"alertEmailEnable": false, "delayEnable": false, "delay": float64(60)},
		"eventEmailSetting": map[string]any{"eventEmailEnable": false, "delayEnable": false, "delay": float64(60)},
		"webhookSetting":    map[string]any{"webhookEnable": false},
		"recipients":        []any{},
		"alertNotifications": []any{
			map[string]any{"key": "OSW_DET_STORM", "shortMsg": "Switch Detected Storm", "module": "Device",
				"email": true, "webhook": false, "enable": true, "level": "Warning", "deviceTypes": []any{"switch"}},
			map[string]any{"key": "OSW_DET_LOOP", "shortMsg": "Switch Detected Loop", "module": "Device",
				"email": false, "webhook": false, "enable": true, "level": "Warning", "deviceTypes": []any{"switch"}},
		},
		"eventNotifications": []any{
			map[string]any{"key": "DEV_IP_C", "shortMsg": "Device IP Changed", "module": "Device",
				"email": false, "webhook": false, "enable": false, "deviceTypes": []any{"ap", "gateway"}},
		},
	}
	auditNotifications := map[string]any{
		"webhookSetting": map[string]any{"webhookEnable": false},
		"logNotifications": []any{
			map[string]any{"key": "AUTHENTICATION", "shortMsg": "Authentication", "webhook": false},
			map[string]any{"key": "CLIENTS", "shortMsg": "Clients", "webhook": false},
		},
	}
	notifDoc := func(store map[string]any) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !requireToken(w, r) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			switch r.Method {
			case http.MethodGet:
			case http.MethodPatch:
				var in map[string]any
				_ = json.NewDecoder(r.Body).Decode(&in)
				if len(in) == 0 {
					writeEnvelope(w, -1001, "Invalid request parameters.", nil)
					return
				}
				for k, v := range in {
					store[k] = v
				}
			default:
				writeEnvelope(w, -1600, "Unsupported request path.", nil)
				return
			}
			out := map[string]any{"resource": float64(0)}
			for k, v := range store {
				out[k] = v
			}
			writeEnvelope(w, 0, "", out)
		}
	}
	mux.HandleFunc("/abc123/api/v2/sites/site-1/logs/notification", notifDoc(notifications))
	mux.HandleFunc("/abc123/api/v2/sites/site-1/site/audit-notification", notifDoc(auditNotifications))
	mux.HandleFunc("/debug/notifications", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"logs": notifications, "audit": auditNotifications})
	})

	mux.HandleFunc("/debug/singletons", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(singletons)
	})

	// Unauthenticated debug endpoints so tests can assert on the RAW stored
	// objects — specifically that keys the provider never models (STP
	// `instances`, and the WiFi `pskSetting.securityKey`) survive updates.
	mux.HandleFunc("/debug/ssids", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(ssids)
	})
	mux.HandleFunc("/debug/profiles", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(w).Encode(profs)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// rawStore fetches one of the mock's debug endpoints (e.g. "ssids", "profiles").
func rawStore(t *testing.T, base, kind string) map[string]map[string]any {
	t.Helper()
	resp, err := http.Get(base + "/debug/" + kind)
	if err != nil {
		t.Fatalf("debug fetch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out := map[string]map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("debug decode: %v", err)
	}
	return out
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
