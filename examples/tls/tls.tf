terraform {
  required_providers {
    temporal = {
      source = "platacard/temporal",
      version = "0.2.0"
    }
  }
}

provider "temporal" {
  host = "127.0.0.1"
  port = "7233"
  
  # Add certs for mTLS auth.
  tls {
    cert_file = sensitive(file("path/to/cert.pem"))
    key_file  = sensitive(file("path/to/key.pem"))
    ca_certs = sensitive(file("path/to/cacerts.pem"))
    server_name = "server-name"
  }
}

# Manage an example namespace.
resource "temporal_namespace" "example" {
  name        = "example"
  description = "This is example namespace"
  owner_email = "admin@example.com"
}
