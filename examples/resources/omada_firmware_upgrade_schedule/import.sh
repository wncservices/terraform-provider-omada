# Import by schedule id. The web UI does not show it; list the schedules with
# GET /{omadacId}/api/v2/upgrade/autoCheck to find it.
terraform import omada_firmware_upgrade_schedule.weekly 0123456789abcdef01234567
