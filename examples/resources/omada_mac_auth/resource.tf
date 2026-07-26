# MAC-based authentication for clients that cannot do 802.1X.
#
# A MAC address travels in clear on every frame and is trivially spoofed, so
# treat this as access-control convenience, not a security boundary — put the
# devices on a segmented network as well.
#
# Needs a RADIUS server to authenticate against; see omada_radius_profile.
resource "omada_mac_auth" "this" {
  enable    = false
  auth_type = 0

  ssid_ids = [
    omada_wireless_network.iot.id,
  ]
}
