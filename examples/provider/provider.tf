terraform {
  required_providers {
    temporal = {
      source = "platacard/temporal"
    }
  }
}

provider "temporal" {
  host     = "127.0.0.1"
  port     = "7233"
  insecure = true
}

# Manage an example namespace.
resource "temporal_namespace" "example" {
  name        = "example"
  description = "This is example namespace"
  owner_email = "admin@example.com"
}
