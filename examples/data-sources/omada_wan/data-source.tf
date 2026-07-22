# WAN settings are read-only — see the data source docs for why.
data "omada_wan" "gateway" {}

output "wan_proto" {
  value = data.omada_wan.gateway.ports[0].proto
}
