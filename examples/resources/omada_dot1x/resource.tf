# Site-wide 802.1X port authentication.
#
# Enabling this disconnects any wired client that cannot authenticate against
# the RADIUS server — which can include the controller itself if it sits behind
# an affected switch port. The RADIUS profile and per-port 802.1X settings are
# not manageable from this provider yet, so stage those in the controller UI
# first and flip `enable` last.
resource "omada_dot1x" "this" {
  enable      = false
  vlan_assign = false
}
