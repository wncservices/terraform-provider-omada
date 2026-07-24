# Captive portal for the guest WiFi, gated by one shared password.
#
# `password` is write-only: the controller never returns it, so Terraform cannot
# detect drift on it. Supply it from a secret store rather than hard-coding it.
resource "omada_portal" "guest" {
  name      = "Guest Portal"
  enable    = true
  auth_type = 1 # 1 = simple password, 0 = click-through
  password  = var.guest_portal_password

  # Gate the guest SSID (wireless clients).
  ssid_ids = [omada_wireless_network.guest.id]
}
