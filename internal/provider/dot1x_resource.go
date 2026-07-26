// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_dot1x manages site-wide 802.1X port authentication
// (Settings -> Authentication -> 802.1X).
//
// This is the site-level switch and its global options only. The RADIUS server
// it authenticates against is a separate object — see `omada_radius_profile` —
// and per-port 802.1X control belongs to the switch port configuration, which
// is not modelled yet (DESIGN.md §5.5). So this resource alone is not
// sufficient for a working deployment.
func NewDot1XResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "dot1x",
		doc:      omada.Dot1XSetting,
		desc: "Manages site-wide 802.1X port authentication.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.\n\n" +
			"~> **Enabling 802.1X can lock wired clients off the network.** Any device that cannot " +
			"authenticate against the configured RADIUS server loses its connection, which on a " +
			"switch uplink can include the controller itself. The RADIUS profile and per-port " +
			"802.1X settings are not yet manageable from this provider, so treat `enable = true` " +
			"as the last step of a change you have already staged in the controller UI.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Enable 802.1X authentication site-wide."},
			{"auth_mode", "authMode", kindInt, "Authentication mode (controller enum; `1` is the default)."},
			{"auth_type", "authType", kindInt, "Authentication type (controller enum; `1` is the default)."},
			{"mac_format", "macFormat", kindInt, "MAC address format used towards the RADIUS server (controller enum)."},
			{"vlan_assign", "vlanAssign", kindBool, "Allow RADIUS to assign the client's VLAN."},
		},
	})
}
