# Needs openapi_client_id / openapi_client_secret on the provider: this
# document is served only by the Open API, so even reading it requires them.
resource "omada_iot_radio" "this" {
  enable     = true
  aging_time = 30

  # Write-only: sent to the controller, never stored in state. Omit it to leave
  # the controller's existing passcode alone.
  passcode = var.iot_passcode
}
