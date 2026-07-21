# Import by network ID (uses the controller's primary site):
terraform import omada_network.iot 692c13f575ee724076c80d2d

# Or scope to a named site: "<site_name>/<network_id>"
terraform import omada_network.iot 'Home/692c13f575ee724076c80d2d'
