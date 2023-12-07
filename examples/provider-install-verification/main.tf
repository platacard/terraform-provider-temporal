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

output "namespace" {
  value = data.temporal_namespace.test
}
