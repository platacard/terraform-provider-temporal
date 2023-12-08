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

data "temporal_namespace" "test" {
  name = "test"
}

resource "temporal_namespace" "hurma" {
  name = "hurma"
}
output "namespace" {
  value = data.temporal_namespace.test
}

output "new_namespace" {
  value = temporal_namespace.hurma
}
