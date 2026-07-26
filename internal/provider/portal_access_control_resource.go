// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_portal_access_control manages the captive portal's pre-authentication
// and free-authentication switches
// (Settings -> Authentication -> Access Control).
//
// Only the two switches are managed. The policy lists themselves —
// `preAuthAccessPolicies` and `freeAuthClientPolicies` — are preserved but not
// modelled: both are empty on the hardware this was developed against, so the
// per-policy shape is unknown and modelling it would be guesswork.
func NewPortalAccessControlResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "portal_access_control",
		doc:      omada.PortalAccessControlSetting,
		desc: "Manages the captive portal's access-control switches: whether clients may reach " +
			"selected destinations *before* authenticating, and whether selected clients skip " +
			"authentication entirely.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **Only the switches are managed.** The policy lists that say *which* destinations and " +
			"clients are exempt are preserved on update but not modelled — they were empty on the " +
			"hardware this was built against, so their shape is unknown. Enabling a switch with no " +
			"policies behind it therefore does nothing; add the policies in the controller UI, or " +
			"capture one so the lists can be modelled properly.",
		fields: []settingField{
			{"pre_auth_access_enable", "preAuthAccessEnable", kindBool, "Allow unauthenticated clients to reach the pre-auth destinations."},
			{"free_auth_client_enable", "freeAuthClientEnable", kindBool, "Allow the listed clients to bypass portal authentication."},
		},
	})
}
