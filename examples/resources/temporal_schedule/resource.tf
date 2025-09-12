# Manage an example schedule.
resource "temporal_schedule" "daily_cleanup" {
  namespace   = default
  schedule_id = "daily-cleanup-task"

  spec = {
    intervals = [{
      every  = "24h"
      offset = "3h"
    }]

    calendar_items = [{
      year         = "2025"
      month        = "1,3,7,11"
      day_of_month = "1,11"
      hour         = "11-14"
    }]

    time_zone = "UTC"
  }

  state = {}

  policy_config = {
    catchup_window = "1m"
  }

  action = {
    workflow = {
      workflow_type = "CleanupWorkflow"
      task_queue    = "cleanup-queue-1"
      workflow_id   = "cleanup-wf-test"

      input = jsonencode({
        retention_days = 30
        dry_run        = false
      })

      execution_timeout = "1h"
      run_timeout       = "30m"
      task_timeout      = "10s"

      retry_policy = {
        initial_interval    = "1s"
        backoff_coefficient = 2.0
        maximum_interval    = "100s"
        maximum_attempts    = 5
      }
    }
  }
}
