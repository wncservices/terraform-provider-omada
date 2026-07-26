# SNMP on the site's devices.
#
# v3 only: v1/v2c sends its community string in cleartext and grants read
# access to device configuration to anyone who can reach the port.
#
# Both credentials are write-only — supplied on apply, never stored in state
# or plan. Source them from a secret store.
resource "omada_snmp" "this" {
  v1_v2c_enable = false

  v3_enable   = true
  v3_username = "monitoring"
  v3_password = var.snmp_v3_password # write-only

  security_level = 1
  auth_mode      = 1
  privacy_mode   = 1
}
