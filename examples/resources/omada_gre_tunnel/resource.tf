# GRE encapsulates but does not encrypt: only enable this over a link that is
# already protected.
resource "omada_gre_tunnel" "this" {
  enable   = false
  ssid_ids = []
}
