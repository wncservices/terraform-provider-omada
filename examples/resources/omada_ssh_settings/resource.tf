# SSH access to the managed devices (gateway, switches, APs).
# This does not affect SSH to the controller itself.
resource "omada_ssh_settings" "this" {
  ssh_enable      = false
  ssh_server_port = 22

  # Only reachable from the device's own subnet.
  layer3_access = false
}
