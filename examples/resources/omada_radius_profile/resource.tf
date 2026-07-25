# RADIUS servers for 802.1X / MAC auth / WPA-Enterprise.
#
# shared_secret is a Terraform write-only attribute: it is sent on apply and
# never persisted to state or plan. Source it from a secret store; do not
# hard-code it.
resource "omada_radius_profile" "corp" {
  name = "corp-radius"

  # Mitigates the BlastRADIUS class of attack. Enable unless a legacy client
  # cannot cope.
  require_message_authenticator = true

  auth_server {
    ip            = "10.10.99.5"
    port          = 1812
    shared_secret = var.radius_secret # write-only
  }

  # A second server is used when the first does not answer.
  auth_server {
    ip            = "10.10.99.6"
    shared_secret = var.radius_secret
  }
}
