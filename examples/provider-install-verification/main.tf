terraform {
  required_providers {
    temporal = {
      source = "hashicorp.com/ganievs/temporal"
    }
  }
}

provider "temporal" {
  host = "127.0.0.1"
  port = "7233"
}
