terraform {
  required_providers {
    temporal = {
      source = "hashicorp.com/ganievs/temporal"
    }
  }
}

provider "temporal" {
  host  = "127.0.0.1"
  port  = "7233"
  token = "dummy"
}

# data "temporal_namespace" "test" {
#   name = "test"
# }

resource "temporal_namespace" "hurma" {
  name                = "hurma"
  description         = "This is a test description edited"
  is_global_namespace = false
  owner_email         = "test1233@dif.tech"
}
resource "temporal_namespace" "test" {
  name        = "asdf"
  description = "This is a test description"
}
output "one_new_namespace" {
  value = temporal_namespace.test
}
# output "namespace" {
#   value = data.temporal_namespace.test
# }

output "new_namespace" {
  value = temporal_namespace.hurma
}
