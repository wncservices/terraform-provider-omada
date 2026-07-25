# Intrusion prevention / detection on the gateway.
#
# Signature matching costs throughput, which is the usual reason to turn the
# protection level down — make that a deliberate choice rather than a default.
resource "omada_ips" "this" {
  enable           = true
  protection_level = 3
  geo_blocking     = true
}

# What each protection level actually covers is reported by the controller and
# cannot be set here.
output "ips_high_categories" {
  value = omada_ips.this.high_categories
}
