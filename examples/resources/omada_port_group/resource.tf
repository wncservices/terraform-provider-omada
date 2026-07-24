# A reusable port group, referenced from a firewall ACL via source/destination
# type 2.
resource "omada_port_group" "web" {
  name  = "web-ports"
  ports = ["80", "443", "8080-8090"]
}
