resource "omada_wireless_network" "iot" {
  wlan_group_id = omada_wlan_group.iot.id
  name          = "IoT"
  psk           = var.iot_wifi_password # sensitive
  vlan_enable   = true
  vlan_id       = 30
}
