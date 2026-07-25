# A reusable schedule profile. Other objects reference a time range rather than
# carrying their own schedule, so this is the piece that makes an SSID schedule
# or a time-bounded firewall rule possible.
resource "omada_time_range" "night_run" {
  name = "night run"

  monday    = true
  tuesday   = true
  wednesday = true
  thursday  = true
  friday    = true
  saturday  = true
  sunday    = true

  # 03:00 -> 06:00
  time_slots {
    start_hour = 3
    end_hour   = 6
  }
}

# Several windows per profile are allowed.
resource "omada_time_range" "office_hours" {
  name = "office hours"

  monday    = true
  tuesday   = true
  wednesday = true
  thursday  = true
  friday    = true

  time_slots {
    start_hour = 9
    end_hour   = 12
  }

  time_slots {
    start_hour   = 12
    start_minute = 30
    end_hour     = 17
  }
}
