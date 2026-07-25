# Pin a client to a fixed address.
#
# Write the MAC in the controller's canonical form (upper-case, dash-separated)
# for a clean plan. Other spellings are accepted and are never treated as a
# different device, but they show up as a one-time cosmetic update.
resource "omada_dhcp_reservation" "homeassistant" {
  network_id = "692c13f575ee724076c80d2e"
  mac        = "DC-A6-32-81-56-08"
  ip         = "10.10.20.10"
  name       = "homeassistant"
}
