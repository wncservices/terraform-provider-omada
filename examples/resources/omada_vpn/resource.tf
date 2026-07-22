# Import an existing VPN and manage its enabled state.
resource "omada_vpn" "home" {
  name   = "Home"
  enable = true
}
