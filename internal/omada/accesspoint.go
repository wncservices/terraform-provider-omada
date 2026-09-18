// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Access point device settings (Devices -> an EAP -> Config).
//
// Verified against a live v6.2.14.11 controller with an EAP670(US) v2.0:
//
//	GET   /sites/{site}/eaps/{mac}   full device document
//	PATCH /sites/{site}/eaps/{mac}   partial update (PUT/POST answer -1600)
//
// Like the gateway and unlike the switch, the AP needs no Open API: the web
// API serves both halves. PATCH genuinely is partial — a body carrying one key
// left radioSetting2g, radioSetting5g, ledSetting, lldpEnable, wlanId,
// lbSetting2g, rssiSetting5g, qosSetting5g and ipSetting byte-identical.
//
// The AP's fields fall into three classes, and conflating them is how this
// resource would break a live site.
//
// # 1. Safe to write
//
// LEDSetting, LLDPEnable, SNMP, L3Access, OFDMA, LoadBalance, RSSI and QoS all
// apply without disturbing associated clients.
//
// The §4 idempotent probe was not enough to establish that. Writing a field its
// own value proves the controller ACCEPTS it, not that it APPLIES it — and this
// endpoint has at least one field (Channel, below) that accepts and ignores. So
// these were verified by changing a value and re-reading: ledSetting 2 -> 1 and
// ofdmaEnable2g true -> false both took effect, and were restored afterwards.
//
// # 2. Applied, but DISRUPTIVE
//
// RadioSetting2G and RadioSetting5G. **Writing a radio object bounces that
// radio even when every value in it is identical to what the device already
// has.** Observed on a live AP: writing radioSetting5g back to itself dropped
// every 5GHz client. They reconnected within seconds, but one laptop fell back
// to 2.4GHz and stayed there.
//
// So the idempotent probe that validates every other field in this provider is
// itself an outage here, and a resource must never write a radio object it has
// not been asked to change. UpdateAccessPoint does not decide that; the
// resource layer compares plan against state and omits unchanged radios.
//
// Within a radio object, TXPower/TXPowerLevel and ChannelWidth do take effect
// (verified: level 3 at 20 dBm applied, 40 MHz applied, both restored).
//
// # 3. Accepted, reported success, and silently ignored
//
// Channel. Sending "36" or 36, with the rest of the radio object intact,
// returns errorCode 0 and leaves channel at "0" (auto). This is §5.5a's "a
// create that lies about failing": a resource modelling Channel as writable
// would show a clean apply and then drift forever. It is therefore read-only.
//
// This was checked twice over. The sibling provider
// emanuelbesliu/terraform-provider-tplink-omada writes radios through a
// dedicated PUT /eaps/{mac}/config/radios instead, and models channel as
// settable. That endpoint does exist here — GET answers -1600 but PUT returns
// errorCode 0, the same write-only shape as the switch port's Open API half —
// and channel STILL does not apply through it. So on this controller and
// device the field is inert on both paths, whatever it does elsewhere.
//
// Firmware, model or a site-level auto-RF setting may explain the difference;
// none of the obvious /setting/{rf,wlanOptimization,rfPlanning} paths exist on
// 6.2.14.11, so the real mechanism is still unmapped.
//
// IPSetting is rejected outright (-1001) in the shape the read returns, so it
// is not modelled either.
type AccessPoint struct {
	MAC   string `json:"mac"`
	Name  string `json:"name"`
	Model string `json:"model"`
	IP    string `json:"ip"`

	// LEDSetting is the controller enum for the device LED; 2 means "follow
	// the site setting".
	LEDSetting int `json:"ledSetting"`

	// LLDPEnable is an enum on APs, not the gateway's bool.
	LLDPEnable int `json:"lldpEnable"`

	RadioSetting2G *RadioSetting `json:"radioSetting2g,omitempty"`
	RadioSetting5G *RadioSetting `json:"radioSetting5g,omitempty"`

	LoadBalance2G *LoadBalanceSetting `json:"lbSetting2g,omitempty"`
	LoadBalance5G *LoadBalanceSetting `json:"lbSetting5g,omitempty"`

	RSSI2G *RSSISetting `json:"rssiSetting2g,omitempty"`
	RSSI5G *RSSISetting `json:"rssiSetting5g,omitempty"`

	QoS2G *QoSSetting `json:"qosSetting2g,omitempty"`
	QoS5G *QoSSetting `json:"qosSetting5g,omitempty"`

	OFDMAEnable2G bool `json:"ofdmaEnable2g"`
	OFDMAEnable5G bool `json:"ofdmaEnable5g"`

	L3Access *struct {
		Enable bool `json:"enable"`
	} `json:"l3AccessSetting,omitempty"`

	SNMP *struct {
		Location string `json:"location"`
		Contact  string `json:"contact"`
	} `json:"snmp,omitempty"`

	// MVLANNetworkID is read-only here on purpose; see UpdateAccessPoint.
	MVLANEnable    bool   `json:"mvlanEnable"`
	MVLANNetworkID string `json:"mvlanNetworkId,omitempty"`
}

// RadioSetting is one radio's configuration. Channel is read-only: see the
// type comment on AccessPoint.
type RadioSetting struct {
	RadioEnable bool `json:"radioEnable"`
	// ChannelWidth and Channel are strings in the controller's document even
	// though both are numeric.
	ChannelWidth string `json:"channelWidth"`
	Channel      string `json:"channel"`
	TXPower      int    `json:"txPower"`
	TXPowerLevel int    `json:"txPowerLevel"`
	Freq         int    `json:"freq"`
	WirelessMode int    `json:"wirelessMode"`
}

type LoadBalanceSetting struct {
	LBEnable   bool `json:"lbEnable"`
	MaxClients int  `json:"maxClients"`
}

type RSSISetting struct {
	RSSIEnable bool `json:"rssiEnable"`
	Threshold  int  `json:"threshold"`
}

type QoSSetting struct {
	WMMEnable         bool `json:"wmmEnable"`
	NoAcknowledgement bool `json:"noAcknowledgement"`
	DeliveryEnable    bool `json:"deliveryEnable"`
}

func accessPointPath(siteID, mac string) string {
	return fmt.Sprintf("/sites/%s/eaps/%s", siteID, NormalizeMAC(mac))
}

// GetAccessPoint returns the AP's device document.
func (c *Client) GetAccessPoint(ctx context.Context, siteID, mac string) (*AccessPoint, error) {
	var ap AccessPoint
	if err := c.Do(ctx, http.MethodGet, accessPointPath(siteID, mac), nil, &ap); err != nil {
		return nil, fmt.Errorf("reading access point %s: %w", mac, err)
	}
	return &ap, nil
}

// UpdateAccessPoint applies a partial set of device settings.
//
// The caller passes only what it means to change. Nothing is read and merged
// first: PATCH is already partial, and echoing back a document read a moment
// ago is how a concurrent change in the UI gets reverted.
//
// Two keys are refused outright.
//
// "channel" inside a radio object is accepted by the controller, reported as
// success, and ignored (see the AccessPoint type comment). Letting it through
// would produce an apply that looks clean and drifts forever, so it is an
// error rather than a silent no-op.
//
// "mvlanEnable" destroys a coupled field. Writing it with the device's own
// current value cleared mvlanNetworkId from a real id to null, and writing the
// id back afterwards returned errorCode 0 without restoring it. Until the pair
// is understood, sending mvlanEnable without an explicit mvlanNetworkId in the
// same body is refused.
func (c *Client) UpdateAccessPoint(ctx context.Context, siteID, mac string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}

	if _, ok := fields["mvlanEnable"]; ok {
		if _, paired := fields["mvlanNetworkId"]; !paired {
			return fmt.Errorf("refusing to write %q without %q: on a live EAP670 "+
				"writing mvlanEnable alone cleared mvlanNetworkId and it could not "+
				"be written back", "mvlanEnable", "mvlanNetworkId")
		}
	}

	for _, key := range []string{"radioSetting2g", "radioSetting5g"} {
		radio, ok := fields[key].(map[string]any)
		if !ok {
			continue
		}
		if _, hasChannel := radio["channel"]; hasChannel {
			return fmt.Errorf("refusing to write %q.channel: the controller accepts "+
				"it, reports success, and leaves the channel unchanged", key)
		}
	}

	if err := c.Do(ctx, http.MethodPatch, accessPointPath(siteID, mac), fields, nil); err != nil {
		return fmt.Errorf("updating access point %s: %w", mac, err)
	}
	return nil
}
