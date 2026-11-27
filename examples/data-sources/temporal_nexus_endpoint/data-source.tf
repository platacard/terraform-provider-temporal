# Get data of the example Nexus endpoint
data "temporal_nexus_endpoint" "example" {
  name = "example-worker"
}

output "example_nexus_endpoint_id" {
  value = data.temporal_nexus_endpoint.example.id
}
