# Look up an existing Nexus endpoint by name.
data "temporal_nexus_endpoint" "bilrost_notegen" {
  name = "bilrost-notegen"
}

output "endpoint_id" {
  value = data.temporal_nexus_endpoint.bilrost_notegen.id
}

output "endpoint_url_prefix" {
  value = data.temporal_nexus_endpoint.bilrost_notegen.url_prefix
}
