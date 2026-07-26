# UPnP on the gateway.
#
# Off is the safer default: UPnP lets any LAN device open inbound ports on
# demand, unauthenticated and unrecorded in this configuration. Use an explicit
# omada_port_forward for the few things that genuinely need one.
resource "omada_upnp" "this" {
  enable = false
}
