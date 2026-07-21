resource "omada_ip_group" "lab_networks" {
  name = "lab-networks"
  ip_list = [
    { ip = "10.10.10.0", mask = 24, description = "clients" },
    { ip = "10.10.20.0", mask = 24, description = "services" },
  ]
}
