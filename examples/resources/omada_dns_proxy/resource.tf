# DNS proxy with a customised DNS-over-HTTPS resolver.
#
# Whoever runs the resolver you point at sees every query the network makes, so
# this is a trust decision as much as a configuration one.
resource "omada_dns_proxy" "this" {
  enable = true

  # Select from the firmware's built-in resolvers by `type`. Read the available
  # values from this resource's own `available_default_servers` output.
  enabled_default_server_types = []

  custom_server {
    name = "filtered"
    urls = ["https://dns.example.com/dns-query"]
  }
}

output "builtin_doh_resolvers" {
  value = omada_dns_proxy.this.available_default_servers
}
