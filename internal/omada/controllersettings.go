// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"net/http"
)

// Controller-wide settings (Global Settings -> Controller Settings).
//
// This is the provider's first CONTROLLER-scoped document: there is no site in
// the path, and the resource built on it carries no `site` attribute. Nothing
// in the provider required one — the provider's own `site` is optional and only
// consumed by resources that choose to resolve it — so controller-scoped
// resources simply do not opt in.
//
//	GET   /controller/setting
//	PATCH /controller/setting
//
// Verified against a live v6.2.14.11 software controller.
//
// # Merge semantics differ per section, so every section is sent whole
//
// PATCH takes a body of named sections. Whether a section may be sent
// *partially* depends on the section, which is not something to guess at:
//
//	webPort, mailServer, deviceAccessManagement, firmware   partial merges
//	general, deviceManage, loggingLevel, certificate        rejected (-1/-1001)
//
// Probed by sending each section a single field carrying its own current value.
// Four returned errorCode 0 and merged; four refused, `general` and
// `loggingLevel` with a bare -1 "General error." and the other two with -1001.
//
// So UpdateControllerSettings takes whole sections. Callers read, modify the
// fields they care about, and send that section complete — which is valid for
// both classes and removes the need to remember which is which. The resource
// layer then sends only the sections that actually changed, so an apply that
// touches nothing writes nothing.
//
// # What is deliberately not modelled
//
// mailServer (SMTP credentials), certificate (key material) and the two RADIUS
// blocks carry secrets or PKI that belong in their own resources if they are
// ever wanted, not in a general settings document.
type ControllerSettings struct {
	General                *ControllerGeneral      `json:"general,omitempty"`
	WebPort                *ControllerWebPort      `json:"webPort,omitempty"`
	DeviceManage           *ControllerDeviceManage `json:"deviceManage,omitempty"`
	DeviceAccessManagement *ControllerDeviceAccess `json:"deviceAccessManagement,omitempty"`
	Firmware               *ControllerFirmware     `json:"firmware,omitempty"`
	LoggingLevel           *ControllerLoggingLevel `json:"loggingLevel,omitempty"`
}

type ControllerGeneral struct {
	Name       string   `json:"name"`
	TimeZone   string   `json:"timeZone"`
	Region     string   `json:"region"`
	NTPEnable  bool     `json:"ntpEnable"`
	NTPServers []string `json:"ntpServers"`
}

// ControllerWebPort carries the controller's own listening ports and, more
// consequentially, HostName.
//
// HostName is the address the controller believes it is reachable at. Left
// unset the controller picks one from the host's interfaces, and it can pick
// badly: on a machine with a WireGuard tunnel it chose the tunnel's address,
// which no adopted device can reach. Pair it with DeviceManage.
type ControllerWebPort struct {
	ManageHTTPPort      int    `json:"manageHttpPort"`
	ManageHTTPSPort     int    `json:"manageHttpsPort"`
	PortalHTTPPort      int    `json:"portalHttpPort"`
	PortalHTTPSPort     int    `json:"portalHttpsPort"`
	HostName            string `json:"hostName"`
	AutoRefresh         bool   `json:"autoRefresh"`
	AutoPortalIPEnable  bool   `json:"autoPortalIpEnable"`
	PortalHTTPSRedirect bool   `json:"portalHttpsRedirect"`
}

// ControllerDeviceManage is the address adopted devices are told to phone home
// to.
//
// With DeviceHostEnable false the controller advertises whatever address it
// inferred, and a device handed an unreachable one goes silently offline some
// time later — the failure looks like a flaky AP, not a configuration error.
// Setting it explicitly is the point of managing this document at all.
type ControllerDeviceManage struct {
	DeviceHostEnable bool   `json:"deviceHostEnable"`
	DeviceHost       string `json:"deviceHost"`
}

type ControllerDeviceAccess struct {
	WebControlHTTP  bool `json:"webControlHttp"`
	WebControlHTTPS bool `json:"webControlHttps"`
	AppDiscovery    bool `json:"appDiscovery"`
}

type ControllerFirmware struct {
	ControllerNotification bool `json:"controllerNotification"`
}

type ControllerLoggingLevel struct {
	Type    string `json:"type"`
	Other   string `json:"other"`
	Manager string `json:"manager"`
	Client  string `json:"client"`
	Device  string `json:"device"`
	Monitor string `json:"monitor"`
	System  string `json:"system"`
	Account string `json:"account"`
	Log     string `json:"log"`
}

const controllerSettingPath = "/controller/setting"

// GetControllerSettings returns the controller settings document.
func (c *Client) GetControllerSettings(ctx context.Context) (*ControllerSettings, error) {
	var out ControllerSettings
	if err := c.Do(ctx, http.MethodGet, controllerSettingPath, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateControllerSettings writes whole sections.
//
// Only the sections present in the body are touched, and each must be complete:
// see the merge-semantics note on ControllerSettings. Passing a body with no
// sections is a no-op rather than an error, so a caller that found nothing to
// change need not special-case it.
func (c *Client) UpdateControllerSettings(ctx context.Context, body map[string]any) error {
	if len(body) == 0 {
		return nil
	}
	return c.Do(ctx, http.MethodPatch, controllerSettingPath, body, nil)
}
