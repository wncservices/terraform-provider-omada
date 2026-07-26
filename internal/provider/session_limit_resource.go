// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_session_limit manages the gateway's session limiting
// (Settings -> Transmission -> Session Limit).
//
// Only the site-wide settings live here. The document also carries a paginated
// `table` of per-host limit rules, which is a separate collection this resource
// deliberately does not touch — the client drops it before writing, so those
// rules survive an update untouched.
func NewSessionLimitResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "session_limit",
		doc:      omada.SessionLimitSetting,
		desc: "Manages the gateway's session limiting: a cap on how many concurrent connections a " +
			"single host may hold.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"Session limiting is a blunt instrument: a cap low enough to contain a misbehaving device " +
			"will also break legitimate heavy users — a BitTorrent client or a browser opening many " +
			"parallel connections can exhaust a modest limit on its own. Raise it deliberately rather " +
			"than accepting a default.\n\n" +
			"~> Per-host limit rules are a separate collection in the same document and are **not** " +
			"managed here; they are preserved untouched on update.",
		fields: []settingField{
			{"enable", "sessionLimitEnable", kindBool, "Whether session limiting is active."},
			{"max_sessions", "sessionLimitMaxSize", kindInt, "Maximum concurrent sessions per host."},
			{"ip_session_enable", "ipSessionEnable", kindBool, "Track sessions per IP address."},
		},
	})
}
