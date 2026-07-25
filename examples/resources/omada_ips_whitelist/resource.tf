# Exempt a network from IPS/IDS inspection.
#
# This switches off a security control for whatever it covers, so reach for it
# to clear a specific false positive — not to quiet a noisy log.
resource "omada_ips_whitelist" "guest" {
  direction      = 1
  traffic_type   = 1 # 1 pairs with a network id
  traffic_source = omada_network.guest.id
}
