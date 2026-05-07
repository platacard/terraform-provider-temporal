# Worker target — route Nexus operations to a handler running in another
# namespace on this self-hosted Temporal cluster.
resource "temporal_nexus_endpoint" "bilrost_notegen" {
  name        = "bilrost-notegen"
  description = "Bilrost note generation"

  worker_target = {
    namespace  = "bilrost"
    task_queue = "notegen-default-queue"
  }
}

# External target — HTTPS URL the server calls to dispatch Nexus operations.
resource "temporal_nexus_endpoint" "external_billing" {
  name        = "external-billing"
  description = "Billing service Nexus operations"

  external_target = {
    url = "https://billing.internal.example.com/nexus"
  }
}
