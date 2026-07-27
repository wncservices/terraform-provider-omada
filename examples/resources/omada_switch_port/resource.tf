# Give a port a name and put it on a VLAN.
#
# Only the attributes listed here are managed — everything else about the port
# keeps whatever the controller currently has. That is what makes it safe to
# adopt a port you have not fully inventoried.
resource "omada_switch_port" "nas" {
  switch_mac = data.omada_devices.all.devices[0].mac
  port       = 4

  name              = "NAS"
  profile_id        = omada_port_profile.main.id
  native_network_id = omada_network.main.id
}

# A trunk to another switch: untagged on management, tagged for the rest.
resource "omada_switch_port" "uplink" {
  switch_mac = data.omada_devices.all.devices[0].mac
  port       = 1

  name = "uplink to core"

  profile_vlan_override_enable = true
  native_network_id            = omada_network.mgmt.id
  network_tags_setting         = 1
  tag_ids = [
    omada_network.main.id,
    omada_network.iot.id,
  ]
}
