# Manage an example namespace.
resource "temporal_namespace" "example" {
  name        = "example"
  description = "This is example namespace"
  owner_email = "admin@example.com"
}

