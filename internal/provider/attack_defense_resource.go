// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_attack_defense manages the gateway's attack-defense settings
// (Settings -> Network Security -> Attack Defense).
//
// Three families of control live in this one document:
//
//   - flood defense — rate-limit new connections per protocol (`*_conn_enable`)
//     or per source (`*_src_enable`);
//   - packet anomaly — drop malformed or hostile packets (ping of death, WinNuke,
//     TCP flag combinations that no legitimate stack sends);
//   - IP options — drop packets carrying IPv4 header options, which are
//     essentially unused on the modern internet and are a classic evasion vector.
//
// The controller reports which sub-features the attached gateway supports via
// `support*` flags; those are read-only and stripped before update.
func NewAttackDefenseResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "attack_defense",
		doc:      omada.AttackDefenseSetting,
		desc: "Manages the gateway's attack-defense settings: flood defense, packet-anomaly " +
			"filtering and IP-options filtering.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it. " +
			"Attributes left unset are not written and keep whatever the controller already has.",
		fields: []settingField{
			// Flood defense: per-protocol connection limits.
			{"tcp_conn_enable", "tcpConnEnable", kindBool, "Limit the rate of new TCP connections (TCP flood defense)."},
			{"udp_conn_enable", "udpConnEnable", kindBool, "Limit the rate of new UDP connections (UDP flood defense)."},
			{"icmp_conn_enable", "icmpConnEnable", kindBool, "Limit the rate of new ICMP connections (ICMP flood defense)."},
			// Flood defense: per-source-address limits.
			{"tcp_src_enable", "tcpSrcEnable", kindBool, "Limit new TCP connections per source address."},
			{"udp_src_enable", "udpSrcEnable", kindBool, "Limit new UDP connections per source address."},
			{"icmp_src_enable", "icmpSrcEnable", kindBool, "Limit new ICMP connections per source address."},

			// Packet anomaly.
			{"tcp_noflag_enable", "tcpNoflagEnable", kindBool, "Drop TCP packets with no flags set."},
			{"tcp_scan_reject", "tcpScanReject", kindBool, "Reject TCP scans. Only effective if the gateway reports support for it."},
			{"tcp_winnuke_enable", "tcpWinnukeEnable", kindBool, "Block WinNuke (out-of-band data) attacks."},
			{"tcp_fin_syn_enable", "tcpFinSynEnable", kindBool, "Drop TCP packets with both FIN and SYN set."},
			{"tcp_fin_noack_enable", "tcpFinNoackEnable", kindBool, "Drop TCP FIN packets without ACK."},
			{"ping_death_enable", "pingDeathEnable", kindBool, "Block ping-of-death (oversized/malformed ICMP echo)."},
			{"ping_large_enable", "pingLargeEnable", kindBool, "Block oversized ping packets."},
			{"ping_wan_enable", "pingWanEnable", kindBool, "Respond to ICMP echo requests arriving on the WAN. Disable to stop the gateway answering pings from the internet."},

			// IPv4 header options.
			{"ip_option_enable", "ipOptionEnable", kindBool, "Filter packets carrying IPv4 header options. Master switch for the ipopt_* settings."},
			{"ipopt_secure_enable", "ipoptSecureEnable", kindBool, "Drop packets with the Security IP option."},
			{"ipopt_loose_route_enable", "ipoptLooseRouteEnable", kindBool, "Drop packets with the Loose Source Route IP option."},
			{"ipopt_strict_route_enable", "ipoptStrictRouteEnable", kindBool, "Drop packets with the Strict Source Route IP option."},
			{"ipopt_record_route_enable", "ipoptRecordRouteEnable", kindBool, "Drop packets with the Record Route IP option."},
			{"ipopt_stream_enable", "ipoptStreamEnable", kindBool, "Drop packets with the Stream ID IP option."},
			{"ipopt_timestamp_enable", "ipoptTimestampEnable", kindBool, "Drop packets with the Timestamp IP option."},
			{"ipopt_noop_enable", "ipoptNoopEnable", kindBool, "Drop packets with the No-Operation IP option."},
		},
	})
}
