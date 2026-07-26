# Captive-portal access control: who may reach what before authenticating.
#
# Only the switches are managed. The policy lists that say WHICH destinations
# and clients are exempt are preserved but not modelled, so enabling a switch
# with no policies behind it does nothing.
resource "omada_portal_access_control" "this" {
  pre_auth_access_enable  = false
  free_auth_client_enable = false
}
