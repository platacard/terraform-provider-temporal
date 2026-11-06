# Worker target — route Nexus operations to a handler running in another
# namespace on this self-hosted Temporal cluster.
resource "temporal_nexus_endpoint" "order_processing" {
  name        = "order-processing"
  description = "Order processing operations handled by the orders namespace"

  worker_target = {
    namespace  = "orders"
    task_queue = "orders-worker"
  }
}

# External target — HTTPS URL the server calls to dispatch Nexus operations.
resource "temporal_nexus_endpoint" "external_payments" {
  name        = "external-payments"
  description = "External payments service Nexus operations"

  external_target = {
    url = "https://payments.example.com/nexus"
  }
}
