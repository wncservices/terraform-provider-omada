resource "omada_lan_dns" "nas" {
  name            = "nas"
  domain          = "nas.wilant.be"
  aliases         = ["storage.wilant.be"]
  ip_addresses    = ["10.10.20.50"]
  lan_network_ids = [omada_network.iot.id] # or existing network IDs
}
