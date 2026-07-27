# IGMP proxy on, IPTV mode off.
#
# Switching a port into IPTV mode takes it out of ordinary service — read the
# available ids from the resource's own `available_ports` output first, and do
# not pick the port carrying your WAN or the switch uplink.
resource "omada_iptv" "this" {
  igmp_proxy_enable = true
  igmp_version      = "2"

  enable           = false
  enabled_port_ids = []
}

output "iptv_ports" {
  value = omada_iptv.this.available_ports
}
