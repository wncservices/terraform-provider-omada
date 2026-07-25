// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// DHCP reservations (Settings -> Services -> DHCP Reservation) pin a client to
// a fixed address.
//
// The item path is keyed on the **MAC address**, not on the object's `id`:
//
//	PUT    /setting/service/dhcp/{mac}
//	DELETE /setting/service/dhcp/{mac}
//
// That is easy to get wrong in a way the API will not tell you about, because
// **a request against an unknown key still answers `errorCode: 0`**. Deleting
// by `id` therefore looks like it worked and silently does nothing. Everything
// here keys on the MAC for that reason; callers must not substitute the id.
//
// The controller also forces `exportToIpMacBinding` to true regardless of what
// is sent, so it is read-only as far as this client is concerned and never
// written.

// DHCPReservation is a static DHCP assignment.
type DHCPReservation struct {
	ID     string `json:"id,omitempty"`
	NetID  string `json:"netId"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Status bool   `json:"status"`
	Name   string `json:"name"`

	// Options are per-reservation DHCP options, preserved as-is.
	Options []any `json:"options"`

	// Read-only. The controller sets this itself; it is never sent.
	ExportToIPMacBinding bool `json:"-"`

	// Derived, read-only display fields.
	NetName    string `json:"netName,omitempty"`
	ClientName string `json:"clientName,omitempty"`
}

// dhcpReservationRaw mirrors the wire shape for reads, including the derived
// keys that must never be written back.
type dhcpReservationRaw struct {
	DHCPReservation
	ExportFlag bool `json:"exportToIpMacBinding"`
}

// NormalizeMAC renders a MAC the way the controller stores it: upper-case, dash
// separated. Accepting colon- or dot-separated input and normalising here stops
// a cosmetic formatting difference showing up as a permanent diff.
func NormalizeMAC(mac string) string {
	r := strings.NewReplacer(":", "-", ".", "-", " ", "")
	return strings.ToUpper(r.Replace(strings.TrimSpace(mac)))
}

func dhcpReservationPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/service/dhcp", siteID)
}

// ListDHCPReservations returns every reservation on the site.
func (c *Client) ListDHCPReservations(ctx context.Context, siteID string) ([]DHCPReservation, error) {
	raws, err := listAll[dhcpReservationRaw](ctx, c, "dhcp reservations", dhcpReservationPath(siteID))
	if err != nil {
		return nil, err
	}
	out := make([]DHCPReservation, 0, len(raws))
	for _, r := range raws {
		item := r.DHCPReservation
		item.ExportToIPMacBinding = r.ExportFlag
		item.MAC = NormalizeMAC(item.MAC)
		out = append(out, item)
	}
	return out, nil
}

// GetDHCPReservationByMAC returns the reservation for a MAC. The MAC is the
// object's real key — see the note at the top of this file.
func (c *Client) GetDHCPReservationByMAC(ctx context.Context, siteID, mac string) (*DHCPReservation, error) {
	want := NormalizeMAC(mac)
	items, err := c.ListDHCPReservations(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].MAC == want {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("dhcp reservation for %q not found", want)
}

// CreateDHCPReservation adds a reservation. The controller answers with the new
// id, but this resolves the stored object by MAC so the caller always gets the
// canonical values back.
func (c *Client) CreateDHCPReservation(ctx context.Context, siteID string, d DHCPReservation) (*DHCPReservation, error) {
	d.ID = ""
	d.MAC = NormalizeMAC(d.MAC)
	if d.Options == nil {
		d.Options = []any{}
	}
	if err := c.Do(ctx, http.MethodPost, dhcpReservationPath(siteID), d, nil); err != nil {
		return nil, fmt.Errorf("creating dhcp reservation: %w", err)
	}
	return c.GetDHCPReservationByMAC(ctx, siteID, d.MAC)
}

// UpdateDHCPReservation updates the reservation keyed by mac. Note the verb is
// PUT and the key is the MAC: PATCH is rejected, and an id in the path is
// accepted-but-ignored.
func (c *Client) UpdateDHCPReservation(ctx context.Context, siteID, mac string, d DHCPReservation) error {
	key := NormalizeMAC(mac)
	d.ID = ""
	d.MAC = NormalizeMAC(d.MAC)
	if d.Options == nil {
		d.Options = []any{}
	}
	path := fmt.Sprintf("%s/%s", dhcpReservationPath(siteID), key)
	if err := c.Do(ctx, http.MethodPut, path, d, nil); err != nil {
		return fmt.Errorf("updating dhcp reservation: %w", err)
	}
	return nil
}

// DeleteDHCPReservation removes the reservation for a MAC.
//
// The controller answers 0 even when nothing matched, so a successful call is
// not evidence the object existed; callers that care must re-read the list.
func (c *Client) DeleteDHCPReservation(ctx context.Context, siteID, mac string) error {
	path := fmt.Sprintf("%s/%s", dhcpReservationPath(siteID), NormalizeMAC(mac))
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting dhcp reservation: %w", err)
	}
	return nil
}
