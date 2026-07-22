resource "omada_port_profile" "trunk" {
  name               = "Trunk"
  poe                = 2 # keep device setting
  native_network_id  = omada_network.mgmt.id
  tagged_network_ids = [omada_network.iot.id, omada_network.services.id]
  vlan_config_enable = true
}
