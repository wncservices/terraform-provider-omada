// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// An iBeacon profile is a BLE advertisement (UUID / major / minor) broadcast by
// IoT-capable access points (Settings -> IoT -> iBeacon).
//
// Verified against a live v6.2 controller:
//
//	GET    /sites/{site}/setting/iot/devices/config       paginated list
//	PUT    /sites/{site}/setting/iot/devices/config/{id}  update (PATCH -> -1600)
//	POST   /sites/{site}/setting/iot/devices/config       create
//	DELETE /sites/{site}/setting/iot/devices/config/{id}  delete
//
// The list and the update verb were confirmed by writing a profile's own values
// back. **Create and delete could not be exercised**: a profile must name at
// least one IoT-capable AP in `macList`, the controller checks that list
// against `/setting/iot/devices`, and in the validation environment that endpoint
// is empty: its access points (EAP610) have no BLE radio, so every create is
// refused with -33284 "The devices in the device list are not in the current
// site". The verb
// and path are the ones the collection accepts (POST answers -1001 for a body
// that is merely incomplete, not -1600), but the success path is unproven.
type IoTBeacon struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`

	Enable bool `json:"enable"`

	// MACList names the access points that broadcast this profile. The
	// controller rejects an empty list, and rejects any device that is not in
	// its own IoT-capable device list.
	MACList []string `json:"macList"`

	// UUID / Major / Minor are the iBeacon identity triple. The controller
	// stores them as hex strings: 32 characters for the UUID, 4 each for major
	// and minor.
	UUID  string `json:"uuid"`
	Major string `json:"major"`
	Minor string `json:"minor"`

	TransmitPower int `json:"transmitPower"`
	// MeasurePower is the calibrated RSSI at one metre, used by receivers to
	// estimate distance. Negative, typically around -65.
	MeasurePower int `json:"measurePower"`
	// AdvIntervalMS is the advertisement interval in milliseconds.
	AdvIntervalMS int `json:"advInterval"`

	// BoundDeviceNum and BuiltIn are controller-owned.
	BoundDeviceNum int `json:"boundDeviceNum"`
	BuiltIn        int `json:"buildIn"`
}

func iotBeaconsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/iot/devices/config", siteID)
}

func iotBeaconPath(siteID, id string) string {
	return fmt.Sprintf("%s/%s", iotBeaconsPath(siteID), id)
}

// ListIoTBeacons returns every iBeacon profile on a site.
func (c *Client) ListIoTBeacons(ctx context.Context, siteID string) ([]IoTBeacon, error) {
	return listAll[IoTBeacon](ctx, c, "iot beacon profiles", iotBeaconsPath(siteID))
}

// GetIoTBeacon returns one profile by id. There is no single-object GET — the
// item path answers -1600 to a GET — so this filters the list.
func (c *Client) GetIoTBeacon(ctx context.Context, siteID, id string) (*IoTBeacon, error) {
	profiles, err := c.ListIoTBeacons(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i], nil
		}
	}
	return nil, fmt.Errorf("iot beacon profile %q not found on site %q", id, siteID)
}

// CreateIoTBeacon creates an iBeacon profile.
//
// Like CreateIoTServer, the list is treated as authoritative rather than the
// response code. That endpoint answers -1 "General error" to a create that in
// fact succeeded, and since both live under /setting/iot the same defence is
// applied here rather than discovering the hard way that this one does it too.
func (c *Client) CreateIoTBeacon(ctx context.Context, siteID string, in IoTBeacon) (*IoTBeacon, error) {
	if len(in.MACList) == 0 {
		return nil, fmt.Errorf("creating iot beacon profile %q: device_macs must name at least one "+
			"access point — the controller refuses an empty device list (-33283)", in.Name)
	}
	in.ID = ""

	var created IoTBeacon
	postErr := c.Do(ctx, http.MethodPost, iotBeaconsPath(siteID), in, &created)
	if postErr == nil && created.ID != "" {
		return &created, nil
	}

	profiles, err := c.ListIoTBeacons(ctx, siteID)
	if err != nil {
		if postErr != nil {
			return nil, fmt.Errorf("creating iot beacon profile %q: %w", in.Name, postErr)
		}
		return nil, fmt.Errorf("iot beacon profile %q was created but could not be read back: %w", in.Name, err)
	}
	for i := range profiles {
		if profiles[i].Name == in.Name {
			return &profiles[i], nil
		}
	}
	if postErr != nil {
		return nil, fmt.Errorf("creating iot beacon profile %q: %w", in.Name, postErr)
	}
	return nil, fmt.Errorf("iot beacon profile %q was created but does not appear in the list", in.Name)
}

// UpdateIoTBeacon writes the modelled fields onto an existing profile.
//
// Read-modify-write, so controller-owned keys and anything not modelled here
// survive. Confirmed live on the site's existing profile: writing its own
// values back is accepted and changes nothing.
func (c *Client) UpdateIoTBeacon(ctx context.Context, siteID, id string, in IoTBeacon) error {
	cur, err := c.RawByID(ctx, iotBeaconsPath(siteID), "id", id)
	if err != nil {
		return err
	}
	for k := range cur {
		if controllerOwnedKey(k) {
			delete(cur, k)
		}
	}
	// The controller maintains these; sending them back is at best noise.
	delete(cur, "boundDeviceNum")
	delete(cur, "buildIn")

	cur["name"] = in.Name
	cur["enable"] = in.Enable
	cur["macList"] = nilToEmptyStrings(in.MACList)
	cur["uuid"] = in.UUID
	cur["major"] = in.Major
	cur["minor"] = in.Minor
	cur["transmitPower"] = in.TransmitPower
	cur["measurePower"] = in.MeasurePower
	cur["advInterval"] = in.AdvIntervalMS

	if err := c.Do(ctx, http.MethodPut, iotBeaconPath(siteID, id), cur, nil); err != nil {
		return fmt.Errorf("updating iot beacon profile %q: %w", id, err)
	}
	return nil
}

// DeleteIoTBeacon removes an iBeacon profile.
func (c *Client) DeleteIoTBeacon(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, http.MethodDelete, iotBeaconPath(siteID, id), nil, nil); err != nil {
		return fmt.Errorf("deleting iot beacon profile %q: %w", id, err)
	}
	return nil
}
