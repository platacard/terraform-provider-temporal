# Limit an Activity task queue and weight fairness keys differently
resource "temporal_task_queue_config" "example_activity" {
  namespace       = "default"
  task_queue      = "example-queue"
  task_queue_type = "Activity"

  queue_rate_limit = {
    requests_per_second = 100
    reason              = "Protect the downstream API"
  }

  fairness_key_rate_limit_default = {
    requests_per_second = 10
  }

  fairness_weight_overrides = {
    "premium-tenant" = 5
    "trial-tenant"   = 0.5
  }
}

# Workflow task queues support fairness weights only
resource "temporal_task_queue_config" "example_workflow" {
  namespace       = "default"
  task_queue      = "example-queue"
  task_queue_type = "Workflow"

  fairness_weight_overrides = {
    "premium-tenant" = 5
  }
}
