# Discover port-forward rule IDs (e.g. to write import blocks).
data "omada_port_forwards" "all" {}

output "port_forward_ids" {
  value = { for r in data.omada_port_forwards.all.port_forwards : r.name => r.id }
}
