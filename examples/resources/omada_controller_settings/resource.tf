# Controller-wide settings. This resource is controller-scoped: unlike every
# other resource in this provider it takes no `site`, and there is exactly one
# of it per controller.
resource "omada_controller_settings" "this" {
  name      = "home"
  time_zone = "America/Los_Angeles"
  region    = "United States"

  # The address adopted devices are told to phone home to.
  #
  # Worth setting explicitly. Left alone, the controller infers one from its
  # host's interfaces, and on a host with a VPN tunnel or several NICs it can
  # pick one the devices cannot reach. Devices then drop off some time later
  # and the symptom looks like flaky hardware.
  device_host_enable = true
  device_host        = "omada.example.com"

  # The address the controller uses for its own links and portal URLs.
  host_name = "omada.example.com"

  # An adopted device's own web UI takes a password, so do not offer it
  # over plain HTTP.
  device_web_control_http  = false
  device_web_control_https = true
}

# Anything you leave out keeps whatever the controller already has and never
# reports drift, so managing two fields is a perfectly good use of this
# resource:
#
#   resource "omada_controller_settings" "this" {
#     device_host_enable = true
#     device_host        = "10.0.0.10"
#   }
#
# Declare only one of these, though. Two would each read the document, write
# their own sections, and fight over anything they both set.
