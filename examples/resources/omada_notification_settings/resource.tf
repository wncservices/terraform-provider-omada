# Alert and event notifications.
#
# The controller knows 131 notifications. `alert` and `event` are sparse maps
# keyed by the controller's own notification key, so you name only the ones you
# care about — everything else keeps whatever the controller has. Omitting a
# toggle inside an entry leaves that toggle alone too.
resource "omada_notification_settings" "this" {
  alert_email_enable = true
  alert_email_delay  = 60

  event_email_enable = false
  webhook_enable     = false

  alert = {
    # A switch reporting a broadcast storm is worth an email.
    OSW_DET_STORM = {
      email  = true
      enable = true
    }
    OSW_DET_LOOP = {
      email  = true
      enable = true
    }
  }

  event = {
    # Noisy on a DHCP network; tracked but not emailed.
    DEV_IP_C = {
      email  = false
      enable = true
    }
  }
}
