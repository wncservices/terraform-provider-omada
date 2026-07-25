// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_ips manages the gateway's intrusion prevention / detection system
// (Settings -> Network Security -> IPS/IDS).
//
// The endpoint returns two different kinds of thing side by side. `enable`,
// `ips_mode`, `geo_blocking`, `protection_level` and `custom_categories` are
// configuration. The four `*_categories` lists are **reference data**: they
// describe which signature categories each protection level covers, and the
// controller preserves them whether or not they are sent. They are therefore
// exposed read-only, so a practitioner can see what a level actually includes
// without being able to "set" something the controller ignores.
func NewIPSResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "ips",
		doc:      omada.IPSSetting,
		desc: "Manages the gateway's IPS/IDS (intrusion prevention and detection).\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.\n\n" +
			"~> **This is an active security control.** Setting `enable = false`, lowering " +
			"`protection_level`, or switching `ips_mode` to detection-only changes what the gateway " +
			"blocks. Signature matching also costs throughput, which is the usual reason people " +
			"turn it down — decide that deliberately rather than by inheriting a default.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Whether IPS/IDS is active."},
			{"ips_mode", "ipsMode", kindInt, "Controller enum selecting prevention versus detection-only. " +
				"TP-Link does not document the mapping and it is not safe to infer from a live system, " +
				"so check the controller UI for which value corresponds to which mode before changing it."},
			{"geo_blocking", "geoEnable", kindBool, "Block traffic by source/destination country (geo-IP filtering)."},
			{"protection_level", "dpLevel", kindInt, "Protection level. `1`, `2` and `3` correspond to the " +
				"`low_categories`, `medium_categories` and `high_categories` sets below; a custom level uses " +
				"`custom_categories`."},
			{"custom_categories", "customCategories", kindIntList, "Signature category IDs to match when a custom " +
				"protection level is selected."},

			// Reference data — reported, never written.
			{"low_categories", "lowCategories", kindIntListRO, "Signature categories covered by the low protection level. Read-only."},
			{"medium_categories", "mediumCategories", kindIntListRO, "Signature categories covered by the medium protection level. Read-only."},
			{"high_categories", "highCategories", kindIntListRO, "Signature categories covered by the high protection level. Read-only."},
			{"all_categories", "allCategories", kindIntListRO, "Every signature category the gateway knows about. Read-only."},
		},
	})
}
