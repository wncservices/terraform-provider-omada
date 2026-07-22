resource "omada_mdns_reflector" "media" {
  name         = "Media to Main"
  profile_ids  = ["buildIn-1"] # AP profile IDs
  service_vlan = "40"          # where services advertise
  client_vlan  = "10"          # where clients discover
}
