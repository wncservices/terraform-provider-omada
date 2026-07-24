# Application Layer Gateway helpers on the gateway.
#
# SIP ALG is left off deliberately: it rewrites addresses inside SIP payloads
# and is a common cause of one-way audio and dropped VoIP registrations. Modern
# SIP endpoints handle NAT traversal themselves.
resource "omada_alg" "this" {
  ftp       = true
  ftp_ports = [21]
  h323      = true
  pptp      = true
  ip_sec    = true

  sip = false
}
