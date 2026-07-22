terraform {
  required_providers {
    omada = {
      source  = "wncservices/omada"
      version = "~> 0.1"
    }
  }
}

# Any attribute may also be supplied via env: OMADA_URL / OMADA_USERNAME /
# OMADA_PASSWORD / OMADA_SITE / OMADA_SKIP_TLS_VERIFY.
provider "omada" {
  url      = "https://10.0.0.2:443" # controller LAN URL
  username = var.omada_username
  password = var.omada_password

  # skip_tls_verify defaults to true (controllers use self-signed certs).
  # site defaults to the controller's primary site; set it only for multi-site
  # controllers, e.g. site = "Home".
}
