# A singleton per site — import by site name.
#
# The `alert` and `event` maps are configuration intent, not discoverable state:
# import cannot know which of the 131 notifications you mean to manage, so
# expect a one-time diff that adds the entries you declared.
terraform import omada_notification_settings.this Home
