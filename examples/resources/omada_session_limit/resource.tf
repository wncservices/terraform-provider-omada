# Cap on concurrent connections per host.
#
# A blunt instrument: a limit low enough to contain a misbehaving device will
# also break legitimate heavy users — a BitTorrent client or a browser opening
# many parallel connections can exhaust a modest cap by itself.
#
# Per-host rules live in the same document but are a separate collection and
# are not managed here; they are preserved on update.
resource "omada_session_limit" "this" {
  enable            = false
  max_sessions      = 128
  ip_session_enable = true
}
