// Copyright (c) wncservices/
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_ssh_settings manages SSH access to the *managed devices* (gateway,
// switches, APs) — Settings -> Services -> SSH. It does not affect SSH to the
// controller itself.
func NewSSHSettingsResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "ssh_settings",
		doc:      omada.SSHSetting,
		desc: "Manages SSH access to the site's managed devices (gateway, switches, access points).\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.\n\n" +
			"Enabling SSH opens a login on every adopted device, authenticated with the site's " +
			"device account. Leave `layer3_access` off unless you actually need to reach devices " +
			"from outside their own subnet.",
		fields: []settingField{
			{"ssh_enable", "sshEnable", kindBool, "Enable SSH on managed devices."},
			{"ssh_server_port", "sshServerPort", kindInt, "TCP port the device SSH server listens on. Controller default is `22`."},
			{"layer3_access", "layer3Access", kindBool, "Allow SSH from outside the device's own subnet."},
		},
	})
}
