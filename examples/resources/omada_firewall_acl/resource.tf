# Permit the management VLAN to reach the services VLAN (gateway ACL).
resource "omada_firewall_acl" "mgmt_to_services" {
  name            = "MGMT to Services"
  policy          = "permit"
  protocols       = [256] # all
  source_ids      = [omada_network.mgmt.id]
  destination_ids = [omada_network.services.id]

  direction = {
    lan_to_lan = true
  }
}
