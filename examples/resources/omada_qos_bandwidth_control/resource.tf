# Bandwidth shaping on one WAN port. Only one rule per WAN port is allowed.
#
# in_bandwidth / out_bandwidth must match the real line rate: the shaper
# divides these figures between the four priority classes, so setting them
# above the true rate makes queueing useless, and below it throttles the link.
#
# Staged disabled — flip `enable` once the figures are right.
resource "omada_qos_bandwidth_control" "wan" {
  wan    = "1_c967cf39292e474291e409b4dfe7f0cd"
  enable = false

  direction     = 2
  in_bandwidth  = 1000000 # kbps
  out_bandwidth = 1000000

  # Four percentages, and the controller requires them to total 100.
  class_ratio = [40, 30, 20, 10]

  out_prioritization    = false
  udp_bandwidth_control = false
}
