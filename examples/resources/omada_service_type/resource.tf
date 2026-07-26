# A reusable protocol/port definition for firewall and QoS rules.
#
# The controller's twelve built-ins (ALL, FTP, SSH, TELNET, …) are reference
# data: reference them by id, but manage only custom ones here.
resource "omada_service_type" "grafana" {
  name              = "grafana"
  protocol          = 0 # what the built-in TCP services use on this firmware
  source_ports      = "0-65535"
  destination_ports = "3000-3000"
  description       = "Grafana UI"
}
