# By profile ID, or "<site>/<id>".
# Shared secrets cannot be imported — the provider never reads them back — so
# re-supply them in configuration after importing.
terraform import omada_radius_profile.corp 6a64ca02bb62a10bd62c4233
