# Uses the provider's default site unless site or site_id is given.
data "omada_networks" "default" {}

output "networks" {
  value = data.omada_networks.default.networks
}
