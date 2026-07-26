# Site-wide MAC filtering switch.
#
# This is only the master toggle — the entries live per-SSID and in filter
# groups the controller does not expose here, so enabling this alone filters
# nothing.
resource "omada_mac_filter" "this" {
  enable = false
}
