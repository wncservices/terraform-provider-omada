# Device-level settings on an adopted access point. Only the attributes set
# here are sent; anything omitted keeps its current value.
#
# Radio attributes are different from the rest: writing a radio document makes
# the controller restart that radio, so changing one drops every client on that
# band for a few seconds. This resource sends a radio only when a value
# actually differs, so re-applying an unchanged configuration is silent.
resource "omada_access_point" "living_room" {
  mac  = data.omada_devices.all.devices[0].mac
  name = "living-room"

  # Applied without restarting a radio.
  led_setting     = 2
  ofdma_enable_5g = true
  snmp_location   = "hallway ceiling"

  # Restarts the 5GHz radio when it changes. Lower power is often better than
  # maximum: it stops the AP shouting further than clients can answer.
  radio_5g_tx_power       = 23
  radio_5g_tx_power_level = 3
}

# radio_5g_channel is read-only. The controller accepts a channel write,
# reports success, and ignores it, so it is exported for reference only:
output "ap_5g_channel" {
  value = omada_access_point.living_room.radio_5g_channel
}
