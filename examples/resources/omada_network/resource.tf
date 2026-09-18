resource "omada_network" "iot" {
  name           = "IoT"
  vlan_id        = 30
  gateway_subnet = "10.10.30.1/24"
  dhcp_enabled   = true
  dhcp_start     = "10.10.30.100"
  dhcp_end       = "10.10.30.250"
  # site is optional — defaults to the controller's primary site.
}

# An L2-only VLAN: a name and a VLAN id, nothing routed.
#
# This is the shape to use on a site with no Omada gateway, where routing, DHCP
# and DNS live on something else. Do not set gateway_subnet or interface_ids
# here — the controller assigns them, and setting either is an error rather
# than a silent no-op.
#
# Networks like these are what a tagged SSID binds to (omada_wireless_network's
# lan_network_id) and what a switch trunk port lists in tagged_network_ids.
resource "omada_network" "cameras" {
  name    = "Cameras"
  vlan_id = 58
  purpose = "vlan"
}
