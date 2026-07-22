resource "omada_network" "iot" {
  name           = "IoT"
  vlan_id        = 30
  gateway_subnet = "10.10.30.1/24"
  dhcp_enabled   = true
  dhcp_start     = "10.10.30.100"
  dhcp_end       = "10.10.30.250"
  # site is optional — defaults to the controller's primary site.
}
