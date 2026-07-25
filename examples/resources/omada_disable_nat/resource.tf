# Route a LAN out of the WAN without NAT.
#
# Staged disabled. Turning this on removes internet access for those networks
# unless the upstream router already has return routes for their subnets —
# without NAT, replies to private addresses have no way back.
#
# The controller allows only ONE disable-NAT rule per WAN port.
resource "omada_disable_nat" "routed_guest" {
  name      = "routed-guest"
  interface = "1_c967cf39292e474291e409b4dfe7f0cd"

  network_ids = [
    "692c13f575ee724076c80d2f",
  ]

  enable = false
}
