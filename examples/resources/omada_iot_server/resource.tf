# A telemetry destination for BLE/IoT observations.
#
# Staged disabled: turning `enable` on starts streaming presence data — which
# devices were near which access point, and when — to the URL below.
resource "omada_iot_server" "telemetry" {
  name       = "presence"
  server_url = "https://telemetry.internal.example/ingest"
  enable     = false

  # Aggregate counts rather than per-device observations, where that suffices.
  count_only     = true
  ssl_tls_enable = true

  device_classes  = [0, 1, 2, 3]
  report_interval = 5
}
