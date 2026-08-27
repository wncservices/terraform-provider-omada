# Give a known client a friendly name without creating a DHCP reservation.
resource "omada_client_alias" "printer" {
  mac   = "60-E9-AA-CC-88-08"
  alias = "Office Printer"
}
