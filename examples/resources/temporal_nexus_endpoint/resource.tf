# Route Nexus requests to a task queue polled by Nexus workers
resource "temporal_nexus_endpoint" "example_worker" {
  name        = "example-worker"
  description = "Operations handled by workers in the default namespace"

  worker_target = {
    namespace  = "default"
    task_queue = "example-nexus-queue"
  }
}

# Route Nexus requests to an external Nexus server
resource "temporal_nexus_endpoint" "example_external" {
  name = "example-external"

  external_target = {
    url = "https://nexus.example.com/services"
  }
}
