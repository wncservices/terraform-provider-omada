# Gateway attack defense.
#
# Only the attributes listed here are managed; anything left unset keeps
# whatever the controller already has, so this can be adopted incrementally.
resource "omada_attack_defense" "this" {
  # Don't answer pings from the internet.
  ping_wan_enable = false

  # Flood defense.
  tcp_conn_enable  = true
  udp_conn_enable  = true
  icmp_conn_enable = true

  # Packet anomaly.
  tcp_noflag_enable    = true
  tcp_winnuke_enable   = true
  tcp_fin_syn_enable   = true
  tcp_fin_noack_enable = true
  ping_death_enable    = true

  # Drop packets carrying IPv4 header options.
  ip_option_enable = true
}
