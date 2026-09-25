# Upgrade the AP and the switch to the newest stable firmware every Sunday at
# 04:15 site time, inside a maintenance window. Devices reboot as they upgrade.
resource "omada_firmware_upgrade_schedule" "weekly" {
  sites       = ["Default"]
  models      = ["EAP670(US) v2.0", "ES205G v1.20"]
  timing_type = 2 # weekly
  day_of_week = 0 # Sunday
  hour        = 4
  minute      = 15
}
