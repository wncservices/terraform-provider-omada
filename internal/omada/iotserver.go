// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// An IoT server is an external endpoint the access points push BLE/IoT
// telemetry to (Settings -> IoT -> Telemetry Servers).
//
// Verified against a live v6.2 controller:
//
//	GET    /sites/{site}/setting/iot/servers       paginated list
//	POST   /sites/{site}/setting/iot/servers       create
//	PUT    /sites/{site}/setting/iot/servers/{id}  update (PATCH answers -1600)
//	DELETE /sites/{site}/setting/iot/servers/{id}  delete
//
// The collection also exists on the Open API (`/openapi/v1/.../setting/iot/servers`,
// pagination required) but the web API serves every operation, so there is no
// reason to cross surfaces here.

// IoTServer is one telemetry destination.
//
// `filters` is deliberately absent. It reads back as an empty object on a server
// created through the UI, so its populated shape has never been observed; a
// guess would be written to a live endpoint. updateIoTServer preserves it
// (along with any other key this struct does not model) by merging into the
// current document rather than replacing it.
type IoTServer struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`

	Enable    bool   `json:"enable"`
	ServerURL string `json:"serverUrl"`

	ServerType int `json:"serverType"`
	FormatType int `json:"formatType"`

	// DeviceClasses selects which classes of device are reported on.
	DeviceClasses []int `json:"deviceClasses"`

	ReportIntervalSeconds int `json:"reportInterval"`

	// Authentication is a controller enum. 99 is what the controller stores for
	// a server with no authentication configured, which is the only value that
	// has been observed — the rest of the enum is not published, and no
	// credential field appears on an unauthenticated server, so a server that
	// needs one cannot be fully configured by this provider yet.
	Authentication int `json:"authentication"`

	RSSIFormat           int  `json:"rssiFormat"`
	CountOnly            bool `json:"countOnly"`
	BLEPeriodicTelemetry bool `json:"blePeriodicTelemetry"`
	RawData              bool `json:"rawData"`

	// SSLTLSEnable is nested under `sslTls` on the wire. It is modelled because
	// this endpoint ships telemetry off-box: without TLS the stream, and any
	// credential the controller later attaches to it, crosses the network in
	// clear.
	SSLTLSEnable bool `json:"-"`
	SSLTLS       *struct {
		Enable bool `json:"enable"`
	} `json:"sslTls,omitempty"`
}

// normalise lifts the nested sslTls flag onto the flat field callers use.
func (s *IoTServer) normalise() {
	if s.SSLTLS != nil {
		s.SSLTLSEnable = s.SSLTLS.Enable
	}
}

func iotServersPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/iot/servers", siteID)
}

func iotServerPath(siteID, id string) string {
	return fmt.Sprintf("%s/%s", iotServersPath(siteID), id)
}

// ListIoTServers returns every configured telemetry server on a site.
func (c *Client) ListIoTServers(ctx context.Context, siteID string) ([]IoTServer, error) {
	servers, err := listAll[IoTServer](ctx, c, "iot servers", iotServersPath(siteID))
	if err != nil {
		return nil, err
	}
	for i := range servers {
		servers[i].normalise()
	}
	return servers, nil
}

// GetIoTServer returns one server by id. The controller has no single-object
// GET here, so this filters the list.
func (c *Client) GetIoTServer(ctx context.Context, siteID, id string) (*IoTServer, error) {
	servers, err := c.ListIoTServers(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range servers {
		if servers[i].ID == id {
			servers[i].normalise()
			return &servers[i], nil
		}
	}
	return nil, fmt.Errorf("iot server %q not found on site %q", id, siteID)
}

// CreateIoTServer creates a telemetry server and returns it as stored.
func (c *Client) CreateIoTServer(ctx context.Context, siteID string, in IoTServer) (*IoTServer, error) {
	in.ID = ""
	in.SSLTLS = &struct {
		Enable bool `json:"enable"`
	}{Enable: in.SSLTLSEnable}
	if len(in.DeviceClasses) == 0 {
		// Checked here rather than left to the controller: its own message is
		// "The parameter Device Class is required", which does not say that an
		// *empty list* counts as missing.
		return nil, fmt.Errorf("creating iot server %q: device_classes must list at least one "+
			"device class — the controller rejects an empty list", in.Name)
	}

	var created IoTServer
	postErr := c.Do(ctx, http.MethodPost, iotServersPath(siteID), in, &created)
	if postErr == nil && created.ID != "" {
		created.normalise()
		return &created, nil
	}

	// The create may have worked even though it reported an error.
	//
	// This endpoint answers -1 "General error" to a create that in fact
	// succeeds — confirmed live, repeatedly: the server appears in the list
	// every time. Trusting the error code would abandon a server that exists,
	// leaving an orphan outside Terraform's knowledge that a later create then
	// collides with (-33249 "This transport stream name already exists").
	//
	// So the list is authoritative, not the response code: look for what was
	// asked for, and only report the original error if it genuinely is not
	// there.
	servers, err := c.ListIoTServers(ctx, siteID)
	if err != nil {
		if postErr != nil {
			return nil, fmt.Errorf("creating iot server %q: %w", in.Name, postErr)
		}
		return nil, fmt.Errorf("iot server %q was created but could not be read back: %w", in.Name, err)
	}
	for i := range servers {
		if servers[i].Name == in.Name {
			servers[i].normalise()
			return &servers[i], nil
		}
	}
	if postErr != nil {
		return nil, fmt.Errorf("creating iot server %q: %w", in.Name, postErr)
	}
	return nil, fmt.Errorf("iot server %q was created but does not appear in the list", in.Name)
}

// UpdateIoTServer writes the modelled fields onto an existing server.
//
// This is a read-modify-write: the current document is fetched raw and the
// modelled keys overwritten, so `filters` — whose populated shape has never
// been observed — and anything else the controller adds survive untouched.
// Replacing the document instead would silently discard them.
func (c *Client) UpdateIoTServer(ctx context.Context, siteID, id string, in IoTServer) error {
	cur, err := c.RawByID(ctx, iotServersPath(siteID), "id", id)
	if err != nil {
		return err
	}
	for k := range cur {
		if controllerOwnedKey(k) {
			delete(cur, k)
		}
	}
	cur["name"] = in.Name
	cur["enable"] = in.Enable
	cur["serverUrl"] = in.ServerURL
	cur["serverType"] = in.ServerType
	cur["formatType"] = in.FormatType
	cur["deviceClasses"] = nilToEmptyInts(in.DeviceClasses)
	cur["reportInterval"] = in.ReportIntervalSeconds
	cur["authentication"] = in.Authentication
	cur["rssiFormat"] = in.RSSIFormat
	cur["countOnly"] = in.CountOnly
	cur["blePeriodicTelemetry"] = in.BLEPeriodicTelemetry
	cur["rawData"] = in.RawData
	// Merge into the existing nested object so any sibling key survives.
	ssl, _ := cur["sslTls"].(map[string]any)
	if ssl == nil {
		ssl = map[string]any{}
	}
	ssl["enable"] = in.SSLTLSEnable
	cur["sslTls"] = ssl

	if err := c.Do(ctx, http.MethodPut, iotServerPath(siteID, id), cur, nil); err != nil {
		return fmt.Errorf("updating iot server %q: %w", id, err)
	}
	return nil
}

// DeleteIoTServer removes a telemetry server.
func (c *Client) DeleteIoTServer(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, http.MethodDelete, iotServerPath(siteID, id), nil, nil); err != nil {
		return fmt.Errorf("deleting iot server %q: %w", id, err)
	}
	return nil
}

// nilToEmptyInts keeps a nil slice from marshalling as null.
func nilToEmptyInts(s []int) []int {
	if s == nil {
		return []int{}
	}
	return s
}
