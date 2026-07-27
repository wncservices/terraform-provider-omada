# An iBeacon advertisement broadcast by the named access points.
#
# Staged disabled: with `enable = true` the APs broadcast continuously, and
# anyone in range can read the UUID/major/minor. Treat the triple as a public
# identifier, not a secret.
resource "omada_iot_beacon" "lobby" {
  name        = "lobby"
  uuid        = "0123456789abcdef0123456789abcdef"
  major       = "0001"
  minor       = "0002"
  enable      = false

  # Must name at least one AP with a BLE radio.
  device_macs = ["60-83-E7-4B-1B-40"]
}
