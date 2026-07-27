# Forward WAN :443 to an ingress controller on the LAN.
resource "omada_port_forward" "https" {
  name          = "ingress"
  external_port = "443"
  forward_ip    = "10.10.20.95"
  forward_port  = "443"
  protocol      = "tcp"
  # wan_port_ids defaults to the WAN port used by existing rules.
}
