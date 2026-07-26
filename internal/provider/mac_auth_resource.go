// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_mac_auth manages MAC-based authentication
// (Settings -> Authentication -> MAC-Based Authentication).
//
// Like `omada_dot1x`, this authenticates against a RADIUS server, so it needs
// an `omada_radius_profile` to be useful.
func NewMACAuthResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "mac_auth",
		doc:      omada.MACAuthSetting,
		desc: "Manages MAC-based authentication: wireless clients are authorised by MAC address " +
			"against a RADIUS server.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **A MAC address is not a credential.** It is transmitted in clear on every frame and " +
			"is trivially spoofed, so this is an access-control convenience for devices that cannot do " +
			"802.1X (printers, IoT), not a security boundary. Pair it with network segmentation rather " +
			"than relying on it. It also needs an `omada_radius_profile` to authenticate against.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Whether MAC-based authentication is active."},
			{"auth_type", "authType", kindInt, "Controller enum for the authentication type."},
			{"ssid_ids", "ssids", kindStringList, "IDs of the SSIDs this applies to (see `omada_wireless_network`)."},
		},
	})
}
