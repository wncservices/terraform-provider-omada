variable "iot_wifi_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "omada_wireless_network" "iot" {
  wlan_group_id = omada_wlan_group.iot.id
  name          = "IoT"
  psk           = var.iot_wifi_password
  psk_revision  = 1 # increment whenever the password changes
  vlan_enable   = true
  vlan_id       = 30
}
