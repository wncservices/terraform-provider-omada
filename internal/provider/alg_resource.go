// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_alg manages the gateway's Application Layer Gateway helpers
// (Settings -> Transmission -> NAT -> ALG).
//
// An ALG inspects a protocol that embeds addresses in its payload and rewrites
// them so the protocol survives NAT. They are convenience features with real
// trade-offs: SIP ALG in particular is a common cause of one-way audio and
// dropped VoIP registrations, and is frequently better left off (it ships
// disabled here) so the endpoint's own NAT traversal can do the job.
func NewALGResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "alg",
		doc:      omada.ALGSetting,
		desc: "Manages the gateway's Application Layer Gateway (ALG) helpers for FTP, H.323, " +
			"PPTP, IPsec and SIP.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.\n\n" +
			"Note that SIP ALG is a frequent cause of VoIP problems; many deployments are better " +
			"off with `sip = false` and NAT traversal handled by the endpoints.",
		fields: []settingField{
			{"ftp", "ftp", kindBool, "FTP ALG."},
			{"ftp_ports", "ftpPorts", kindIntList, "TCP ports the FTP ALG inspects. Controller default is `[21]`."},
			{"h323", "h323", kindBool, "H.323 ALG (legacy video conferencing)."},
			{"pptp", "pptp", kindBool, "PPTP pass-through."},
			{"ip_sec", "ipSec", kindBool, "IPsec pass-through."},

			{"sip", "sip", kindBool, "SIP ALG. Often best left disabled — see the resource description."},
			{"sip_tcp", "sipTcp", kindBool, "Inspect SIP over TCP."},
			{"sip_udp", "sipUdp", kindBool, "Inspect SIP over UDP."},
			{"sip_ports", "sipPorts", kindIntList, "Ports the SIP ALG inspects. Controller default is `[5060, 5061]`."},
			{"sip_direct_signaling", "sipDirectSignaling", kindBool, "Allow direct SIP signalling between endpoints."},
			{"sip_direct_media", "sipDirectMedia", kindBool, "Allow direct SIP media between endpoints."},
			{"sip_timeout", "sipTimeout", kindBool, "Apply the SIP signalling/media timeouts below."},
			{"sip_signaling_timeout", "sipSignalingTimeout", kindInt, "SIP signalling timeout (seconds)."},
			{"sip_media_timeout", "sipMediaTimeout", kindInt, "SIP media timeout (seconds)."},
		},
	})
}
