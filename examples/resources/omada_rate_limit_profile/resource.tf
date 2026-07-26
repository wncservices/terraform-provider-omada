# A per-client throughput cap that SSIDs and portal auth can reference.
#
# A limit only exists on the controller while its enable flag is set, so set
# them together — the provider rejects a limit without its flag at plan time
# rather than writing a value the controller would drop.
resource "omada_rate_limit_profile" "guest" {
  name = "guest-capped"

  download_limit_enable = true
  download_limit        = 20000 # controller units; the UI presents Kbps

  upload_limit_enable = true
  upload_limit        = 5000
}
