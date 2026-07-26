# Which audit-log categories reach the webhook.
#
# Audit entries carry only a `webhook` toggle — no email, no enable.
resource "omada_audit_notification" "this" {
  webhook_enable = true

  log = {
    AUTHENTICATION = { webhook = true }
    LOG            = { webhook = true }
  }
}
