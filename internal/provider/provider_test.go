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
	acls := map[string]map[string]any{}
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
