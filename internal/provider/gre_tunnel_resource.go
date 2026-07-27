// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_gre_tunnel manages the GRE tunnel toggle
// (Settings -> VPN -> GRE Tunnel).
func NewGRETunnelResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "gre_tunnel",
		doc:      omada.GRETunnelSetting,
		desc: "Manages the site's GRE tunnel setting.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **GRE provides no confidentiality or authentication.** It encapsulates, it does not " +
			"encrypt: anyone on the path can read and modify what the tunnel carries. Use it only over " +
			"a link that is already protected (IPsec, or a physically trusted circuit), never straight " +
			"across the internet.",
		fields: []settingField{
			{"enable", "greEnable", kindBool, "Whether GRE tunnelling is active."},
			{"ssid_ids", "relatedSsidList", kindStringList,
				"IDs of the SSIDs carried over the tunnel (see `omada_wireless_network`)."},
		},
	})
}
