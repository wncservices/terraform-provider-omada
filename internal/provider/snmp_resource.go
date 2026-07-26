// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_snmp manages SNMP on the site's devices (Site Settings -> Services ->
// SNMP).
//
// Both credentials here are secrets and both are write-only. That is not
// belt-and-braces: the controller returns the SNMP v3 password in **plaintext**
// on read, exactly like the WiFi psk and a RADIUS shared secret, so storing
// either would put it in the state file. See DESIGN.md §2.6.
//
// The v1/v2c community string and the v3 username/password are separate paths:
// v1/v2c authenticates with the community alone, v3 with a user and password.
// Enabling a version without its credential is rejected by the controller.
func NewSNMPResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "snmp",
		doc:      omada.SNMPSetting,
		desc: "Manages SNMP on the site's devices.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.\n\n" +
			"~> **Prefer v3.** SNMP v1/v2c authenticates with a community string sent in cleartext and " +
			"grants read access to device configuration to anyone who can reach the port. If v1/v2c is " +
			"unavoidable, restrict it at the network layer.\n\n" +
			"`community_string` and `v3_password` are **write-only**: supplied on apply, never persisted " +
			"to state or plan. The controller returns the v3 password in plaintext on read, which is " +
			"exactly why the provider refuses to store it. An update that omits a secret keeps the one " +
			"already configured.",
		fields: []settingField{
			{"v1_v2c_enable", "snmpV1V2CEnable", kindBool, "Enable SNMP v1/v2c. Requires `community_string`."},
			{"community_string", "communityString", kindStringWO, "v1/v2c community string. **Write-only.** " +
				"Visible ASCII, 1–64 characters."},

			{"v3_enable", "snmpV3Enable", kindBool, "Enable SNMP v3. Requires `v3_username` and `v3_password`."},
			{"v3_username", "username", kindString, "SNMP v3 username."},
			{"v3_password", "password", kindStringWO, "SNMP v3 authentication password. **Write-only.**"},

			{"security_level", "securityLevel", kindInt, "v3 security level (controller enum): whether messages are " +
				"authenticated and encrypted."},
			{"auth_mode", "authMode", kindInt, "v3 authentication algorithm (controller enum)."},
			{"privacy_mode", "privacyMode", kindInt, "v3 privacy/encryption algorithm (controller enum). Only " +
				"meaningful at a security level that encrypts."},
		},
	})
}
