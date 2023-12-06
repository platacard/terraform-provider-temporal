terraform {
  required_providers {
    temporal = {
      source = "hashicorp.com/ganievs/temporal"
    }
  }
}

provider "temporal" {}

data "temporal_namespaces" "example" {}

