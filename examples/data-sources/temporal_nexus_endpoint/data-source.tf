# Look up an existing Nexus endpoint by name.
data "temporal_nexus_endpoint" "order_processing" {
  name = "order-processing"
}

output "endpoint_id" {
  value = data.temporal_nexus_endpoint.order_processing.id
}

output "endpoint_url_prefix" {
  value = data.temporal_nexus_endpoint.order_processing.url_prefix
}
