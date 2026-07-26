// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Gateway bandwidth control (Settings -> QoS -> Bandwidth Control) shapes
// traffic on one WAN port.
//
// The controller allows **one rule per WAN port** and rejects a second with
// `-43310 The WAN Port is being used by other bandwidths`. Update is `PUT`
// (`PATCH` → -1600), and create answers with a null result, so the new rule is
// resolved from the list by its WAN.
//
// `wan` takes the controller's `1_<hex>` WAN interface id — the same identifier
// DisableNAT.Interface uses, and the one one-to-one NAT wants in `interfaceIds`.

// QoSBandwidthControl is a bandwidth-control rule for one WAN port.
type QoSBandwidthControl struct {
	ID string `json:"id,omitempty"`
	// WAN is the WAN interface id, in the controller's `1_<hex>` form.
	WAN string `json:"wan"`
	// Status enables shaping. A rule with Status false is inert.
	Status bool `json:"status"`
	// Direction is a controller enum for which way traffic is shaped.
	Direction int `json:"direction"`
	// InBandwidth and OutBandwidth are the link rates the shaper works from.
	InBandwidth  int `json:"inBandwidth"`
	OutBandwidth int `json:"outBandwidth"`
	// ClassRatio is the per-priority-class share, four values summing to 100.
	ClassRatio []int `json:"classRatio"`
	// OutPrioritization enables egress prioritisation.
	OutPrioritization bool `json:"outPrioritization"`
	// UDPBandwidthCtrl extends shaping to UDP.
	UDPBandwidthCtrl bool `json:"udpBandwidthCtrl"`
}

func qosBWCPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/qos/gateway/bwc", siteID)
}

// ListQoSBandwidthControls returns every bandwidth-control rule.
func (c *Client) ListQoSBandwidthControls(ctx context.Context, siteID string) ([]QoSBandwidthControl, error) {
	return listAll[QoSBandwidthControl](ctx, c, "bandwidth controls", qosBWCPath(siteID))
}

// GetQoSBandwidthControl returns one rule by id.
func (c *Client) GetQoSBandwidthControl(ctx context.Context, siteID, id string) (*QoSBandwidthControl, error) {
	items, err := c.ListQoSBandwidthControls(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("bandwidth control %q not found", id)
}

// CreateQoSBandwidthControl creates a rule and returns its id. The controller
// returns a null result, so the rule is resolved by its WAN — which is safe
// because only one rule per WAN port is permitted.
func (c *Client) CreateQoSBandwidthControl(ctx context.Context, siteID string, b QoSBandwidthControl) (string, error) {
	b.ID = ""
	if b.ClassRatio == nil {
		b.ClassRatio = []int{}
	}
	if err := c.Do(ctx, http.MethodPost, qosBWCPath(siteID), b, nil); err != nil {
		return "", fmt.Errorf("creating bandwidth control: %w", err)
	}
	items, err := c.ListQoSBandwidthControls(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.WAN == b.WAN {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created bandwidth control for wan %q but could not resolve its id", b.WAN)
}

// UpdateQoSBandwidthControl updates a rule. Note PUT — PATCH is rejected.
func (c *Client) UpdateQoSBandwidthControl(ctx context.Context, siteID, id string, b QoSBandwidthControl) error {
	b.ID = id
	if b.ClassRatio == nil {
		b.ClassRatio = []int{}
	}
	path := fmt.Sprintf("%s/%s", qosBWCPath(siteID), id)
	if err := c.Do(ctx, http.MethodPut, path, b, nil); err != nil {
		return fmt.Errorf("updating bandwidth control: %w", err)
	}
	return nil
}

// DeleteQoSBandwidthControl removes a rule.
func (c *Client) DeleteQoSBandwidthControl(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", qosBWCPath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting bandwidth control: %w", err)
	}
	return nil
}
