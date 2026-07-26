# Transmission-level bandwidth control on the gateway.
#
# Not the same thing as omada_qos_bandwidth_control, which shapes traffic per
# WAN port under QoS. These are separate endpoints and configure independently.
#
# Per-host rules live in the same document and are preserved, not managed here.
resource "omada_gateway_bandwidth_control" "this" {
  enable            = false
  threshold_enable  = false
  threshold_percent = 80
}
