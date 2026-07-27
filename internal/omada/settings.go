// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Several controller settings are *flat singleton documents*: one JSON object
// per site behind a fixed path, with no id, that cannot be created or deleted —
// only read and updated. SSH, ALG and attack defense are all of this shape.
//
// Two things make them awkward enough to justify a shared helper:
//
//  1. The update verb is not consistent. `/setting/ssh`,
//     `/setting/transmission/alg` and `/setting/firewall/attackdefense` reject
//     PATCH with -1600 and require PUT; `/setting/dot1x` and
//     `/setting/accessControl` are the other way round. Each SettingDoc below
//     records the verb that was confirmed against a live controller.
//  2. Reads carry controller-owned metadata that must not be echoed back — the
//     `support*` / `exist*` capability flags the UI uses to decide what to
//     render, and a `resource` counter. Sending them back is at best noise and
//     at worst rejected, so updateSetting strips them.

// SettingDoc locates a flat singleton settings document and records the verb
// that updates it.
type SettingDoc struct {
	// Path is appended to /sites/{siteID}.
	Path string
	// Verb is the confirmed update method (http.MethodPut or http.MethodPatch).
	Verb string
	// OpenAPI marks a document that lives on the Open API rather than the web
	// API. A few do: `/setting/iot/radio` is served only under /openapi/v1 and
	// answers -1600 on the web API. Such documents need Open API credentials to
	// read as well as to write, unlike omada_switch_port where only the write
	// crosses over.
	OpenAPI bool
	// ReadOnlyKeys are document-specific keys the controller owns, beyond the
	// generic `resource` / `support*` / `exist*` metadata. They are dropped
	// before an update so the provider never writes back data it does not
	// manage. Confirmed safe on hardware: PATCHing only the writable subset is
	// accepted and leaves these untouched.
	ReadOnlyKeys []string
}

// The singleton settings documents this provider manages. Every verb here was
// confirmed against a live v6.2 controller by writing the document's own
// current values back and checking the result was accepted and unchanged.
var (
	SSHSetting           = SettingDoc{Path: "/setting/ssh", Verb: http.MethodPut}
	ALGSetting           = SettingDoc{Path: "/setting/transmission/alg", Verb: http.MethodPut}
	AttackDefenseSetting = SettingDoc{Path: "/setting/firewall/attackdefense", Verb: http.MethodPut}
	// Note the verb: unlike the three above, dot1x takes PATCH and answers
	// -1600 to PUT.
	Dot1XSetting = SettingDoc{Path: "/setting/dot1x", Verb: http.MethodPatch}
	// Wireless MAC filtering. PUT.
	MACFilterSetting = SettingDoc{Path: "/setting/firewall/macfilter", Verb: http.MethodPut}
	// MAC-based authentication. PATCH, like dot1x.
	MACAuthSetting = SettingDoc{Path: "/setting/macAuth", Verb: http.MethodPatch}
	// GRE tunnel VPN. PUT; PATCH and POST both answer -1600.
	GRETunnelSetting = SettingDoc{Path: "/setting/vpns/greTunnel", Verb: http.MethodPut}
	// IoT radio (the Zigbee/BLE radio on the access points).
	//
	// This one is served **only by the Open API** — the web API answers -1600
	// for the same path — so it needs Open API credentials to read as well as
	// to write. PATCH; PUT and POST answer -1600.
	IoTRadioSetting = SettingDoc{Path: "/setting/iot/radio", Verb: http.MethodPatch, OpenAPI: true}
	// UPnP. PUT, and a single field.
	UPnPSetting = SettingDoc{Path: "/setting/upnp", Verb: http.MethodPut}
	// Session limits. PUT. The document also carries a paginated `table` of
	// per-host rules, which is a separate concern and is left untouched by the
	// read-modify-write.
	SessionLimitSetting = SettingDoc{
		Path: "/setting/transmission/sessionLimits", Verb: http.MethodPut,
		ReadOnlyKeys: []string{"table"},
	}
	// Gateway bandwidth control. PUT, and the write shape differs from the
	// read shape: the GET nests the settings under `bandwidthControl` while
	// the PUT wants them flat (the nested form is rejected -1001). The
	// per-host `table` is a separate collection and is dropped before writing.
	GatewayBandwidthControlSetting = SettingDoc{
		Path: "/setting/transmission/bandwidthControls", Verb: http.MethodPut,
		ReadOnlyKeys: []string{"table", "bandwidthControl"},
	}
	// Captive-portal pre-authentication access. PATCH.
	PortalAccessControlSetting = SettingDoc{
		Path: "/setting/accessControl", Verb: http.MethodPatch,
		ReadOnlyKeys: []string{"preAuthAccessPolicies", "freeAuthClientPolicies"},
	}
	// IPS/IDS. PATCH, like dot1x. The *Categories lists describe which
	// signature categories each protection level covers — reference data the
	// controller maintains, not configuration.
	// SNMP. PUT, and note the read returns the v3 password in plaintext — see
	// the write-only handling in the resource.
	SNMPSetting = SettingDoc{Path: "/setting/snmp", Verb: http.MethodPut}
	IPSSetting  = SettingDoc{
		Path: "/setting/ips", Verb: http.MethodPatch,
		ReadOnlyKeys: []string{"lowCategories", "mediumCategories", "highCategories", "allCategories"},
	}
)

func (d SettingDoc) path(siteID string) string {
	return fmt.Sprintf("/sites/%s%s", siteID, d.Path)
}

// controllerOwnedKey reports whether k is metadata the controller adds to reads
// and that must not be sent back on update.
func controllerOwnedKey(k string) bool {
	if k == "resource" {
		return true
	}
	for _, p := range []string{"support", "exist", "osgSupport"} {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// GetSetting reads a flat singleton settings document.
func (c *Client) GetSetting(ctx context.Context, siteID string, doc SettingDoc) (map[string]any, error) {
	var out map[string]any
	if err := c.doSetting(ctx, http.MethodGet, siteID, doc, nil, &out); err != nil {
		return nil, fmt.Errorf("reading %s: %w", doc.Path, err)
	}
	return out, nil
}

// doSetting dispatches to whichever API surface serves the document.
func (c *Client) doSetting(ctx context.Context, method, siteID string, doc SettingDoc, body, out any) error {
	if doc.OpenAPI {
		return c.DoOpenAPI(ctx, method, c.OpenAPIPath(siteID, doc.Path), body, out)
	}
	return c.Do(ctx, method, doc.path(siteID), body, out)
}

// UpdateSetting read-modify-writes a flat singleton settings document: it
// merges fields into the document's current value so keys this provider does
// not model survive untouched, strips controller-owned metadata, and sends the
// whole document back with the verb confirmed for that endpoint.
func (c *Client) UpdateSetting(ctx context.Context, siteID string, doc SettingDoc, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	cur, err := c.GetSetting(ctx, siteID, doc)
	if err != nil {
		return err
	}
	for k := range cur {
		if controllerOwnedKey(k) {
			delete(cur, k)
		}
	}
	for _, k := range doc.ReadOnlyKeys {
		delete(cur, k)
	}
	mergeInto(cur, fields)
	if err := c.doSetting(ctx, doc.Verb, siteID, doc, cur, nil); err != nil {
		return fmt.Errorf("updating %s: %w", doc.Path, err)
	}
	return nil
}
