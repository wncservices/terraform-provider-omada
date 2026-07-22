# Route a remote subnet via a next-hop gateway on the LAN.
resource "omada_static_route" "lab" {
  name         = "to-lab"
  destinations = ["192.168.50.0/24"]
  next_hop_ip  = "10.10.20.1"
  metric       = 0
}
