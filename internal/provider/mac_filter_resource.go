// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_mac_filter manages the site-wide MAC filtering switch
// (Settings -> Network Security -> MAC Filter).
//
// Only the master toggle lives in this document. The filter *entries* are held
// per-SSID (`omada_wireless_network.mac_filter_enable`) and in filter groups
// the controller does not expose on this endpoint, so enabling this alone
// filters nothing.
func NewMACFilterResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "mac_filter",
		doc:      omada.MACFilterSetting,
		desc: "Manages the site-wide MAC filtering switch.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> This is only the master toggle. The filter entries themselves are held elsewhere " +
			"(per-SSID, via `omada_wireless_network.mac_filter_enable`, and in filter groups this " +
			"endpoint does not expose), so turning it on by itself filters nothing. As with " +
			"`omada_mac_auth`, remember a MAC address is trivially spoofed — this keeps honest " +
			"devices off a network, it does not keep attackers off one.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Whether MAC filtering is active site-wide."},
		},
	})
}
