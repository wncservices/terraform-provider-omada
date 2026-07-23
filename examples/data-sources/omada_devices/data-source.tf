# Inventory of adopted devices on the site.
data "omada_devices" "all" {}

output "device_inventory" {
  value = { for d in data.omada_devices.all.devices : d.name => "${d.model} @ ${d.ip}" }
}

# Devices with a firmware upgrade available.
output "upgrades_available" {
  value = [for d in data.omada_devices.all.devices : d.name if d.need_upgrade]
}
