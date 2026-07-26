// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_upnp manages UPnP / NAT-PMP on the gateway
// (Settings -> Transmission -> NAT -> UPnP).
func NewUPnPResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "upnp",
		doc:      omada.UPnPSetting,
		desc: "Manages UPnP on the gateway.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **UPnP lets any device on the LAN open inbound ports on the gateway by asking**, " +
			"with no authentication and no record in this configuration. That is convenient for games " +
			"and consoles and is exactly why it is a common lateral-movement aid: a compromised device " +
			"can expose itself, or another host, to the internet. Prefer an explicit " +
			"`omada_port_forward` for the handful of things that genuinely need one.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Whether UPnP is active."},
		},
	})
}
