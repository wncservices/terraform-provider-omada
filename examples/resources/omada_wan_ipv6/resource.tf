# A WAN port's IPv6 connection settings.
#
# Writes here are inferred, not live-validated — see the resource docs. Import
# an existing WAN port (data.omada_wan lists their IDs) and verify a no-op
# plan before changing anything.
resource "omada_wan_ipv6" "wan1" {
  port_uuid  = data.omada_wan.this.ports[0].port_uuid
  enable     = true
  proto      = "dynamic" # the only verified value: DHCPv6/SLAAC autoconfig
  proto_type = 1

  dynamic = {
    get_ipv6      = "auto"
    get_ipv6_type = 3
    prefix        = 1  # request prefix delegation from the ISP
    pd_size       = 48 # requested delegated prefix size, in bits
    dns           = "dynamic"
    dns_type      = 0
  }
}

data "omada_wan" "this" {}
