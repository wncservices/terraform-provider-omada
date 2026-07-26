// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_gateway_bandwidth_control manages the gateway's bandwidth-control
// settings (Settings -> Transmission -> Bandwidth Control).
//
// Not to be confused with `omada_qos_bandwidth_control`
// (/setting/qos/gateway/bwc), which shapes traffic per WAN port. This one is
// the transmission-level control with a utilisation threshold.
func NewGatewayBandwidthControlResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "gateway_bandwidth_control",
		doc:      omada.GatewayBandwidthControlSetting,
		desc: "Manages the gateway's transmission bandwidth control.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **Distinct from `omada_qos_bandwidth_control`.** That resource shapes traffic per WAN " +
			"port under QoS; this one is the transmission-level control with a utilisation threshold. " +
			"They are separate endpoints and can be configured independently.\n\n" +
			"Per-host bandwidth rules live in the same document as a separate collection and are " +
			"**not** managed here; they are preserved untouched on update.",
		fields: []settingField{
			// Dotted keys: the GET nests these under `bandwidthControl`, the PUT
			// takes them flat.
			{"enable", "bandwidthControl.bandwidthControlEnable", kindBool, "Whether bandwidth control is active."},
			{"threshold_enable", "bandwidthControl.thresholdControlEnable", kindBool, "Only apply control once the utilisation threshold is exceeded."},
			{"threshold_percent", "bandwidthControl.thresholdValue", kindInt, "Utilisation threshold, as a percentage of the link rate."},
		},
	})
}
