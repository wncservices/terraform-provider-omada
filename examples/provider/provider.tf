terraform {
  required_providers {
    omada = {
      source = "wncservices/omada"
    }
  }
}

# Credentials may also come from OMADA_URL / OMADA_USERNAME / OMADA_PASSWORD.
provider "omada" {
  url             = "https://10.0.0.2:443"
  username        = var.omada_username
  password        = var.omada_password
  skip_tls_verify = true
  site            = "Default"
}
