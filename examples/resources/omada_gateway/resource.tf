# Device-level settings on the gateway. Only the attributes set here are sent;
# anything omitted keeps its current value.
#
# The gateway's physical ports are not managed here by design — see the
# resource documentation.
resource "omada_gateway" "this" {
  mac  = data.omada_devices.all.devices[0].mac
  name = "border"

  lldp_enable       = false
  hw_offload_enable = true

  snmp_location = "rack 1"
}
