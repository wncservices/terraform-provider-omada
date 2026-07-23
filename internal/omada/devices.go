// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Device is an adopted Omada device (gateway, switch or AP). Verified against a
// live v6.2 controller (GET /sites/{id}/devices). Only stable inventory fields
// are modelled; the endpoint returns many more runtime/telemetry keys.
type Device struct {
	Name            string `json:"name"`
	Type            string `json:"type"`  // "gateway" | "switch" | "ap"
	Model           string `json:"model"` // e.g. "ES205GP"
	CompoundModel   string `json:"compoundModel"`
	MAC             string `json:"mac"`
	SN              string `json:"sn"`
	IP              string `json:"ip"`
	Status          int    `json:"status"`
	StatusCategory  int    `json:"statusCategory"` // 0=disconnected, 1=connected, 2=pending
	FirmwareVersion string `json:"firmwareVersion"`
	Version         string `json:"version"`
	NeedUpgrade     bool   `json:"needUpgrade"`
	UptimeSeconds   int64  `json:"uptimeLong"`
	ClientNum       int    `json:"clientNum"`
	HWVersion       string `json:"hwVersion"`
}

func devicesPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/devices", siteID)
}

// ListDevices returns all adopted devices on a site.
//
// This endpoint returns a bare JSON array in `result` (not the paginated
// {data,totalRows} envelope the /setting/* list endpoints use), so it is decoded
// directly rather than through the pager.
func (c *Client) ListDevices(ctx context.Context, siteID string) ([]Device, error) {
	var devices []Device
	if err := c.Do(ctx, "GET", devicesPath(siteID), nil, &devices); err != nil {
		return nil, fmt.Errorf("listing devices: %w", err)
	}
	return devices, nil
}
