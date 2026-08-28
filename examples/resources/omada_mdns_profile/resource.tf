# A custom mDNS service profile: a named bundle of raw mDNS service strings
# that omada_mdns_reflector.profile_ids can reference, alongside the
# controller's read-only built-ins ("buildIn-1" .. "buildIn-10").
#
# The controller caps custom profiles per site (5 on the firmware this was
# developed against) — bundle related services into one profile rather than
# spending a slot per service where that makes sense, as done here for Matter.
resource "omada_mdns_profile" "matter" {
  name        = "matter"
  service_ids = ["_matter._tcp.local", "_matterc._udp.local"]
}
