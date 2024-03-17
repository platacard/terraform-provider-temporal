terraform {
  required_providers {
    temporal = {
      source = "platacard/temporal"
    }
  }
}

provider "temporal" {
  host = "127.0.0.1"
  port = "7233"
  
  # Add certs for mTLS auth.
  tls {
    cert = sensitive(file("path/to/cert.pem"))
    key  = sensitive(file("path/to/key.pem"))
    ca = sensitive(file("path/to/cacerts.pem"))
    server_name = "server-name"
  }
}

# Manage an example namespace.
resource "temporal_namespace" "example" {
  name        = "example"
  description = "This is example namespace"
  owner_email = "admin@example.com"
}
