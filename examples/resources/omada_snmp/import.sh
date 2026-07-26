# SNMP settings are a singleton per site — import by site name.
#
# The write-only credentials cannot round-trip through import; re-supply them
# in configuration afterwards. Omitting them keeps what the controller has.
terraform import omada_snmp.this Home
