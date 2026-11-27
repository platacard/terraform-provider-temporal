# Get data of the example schedule
data "temporal_schedule" "example" {
  namespace   = "default"
  schedule_id = "daily-cleanup-task"
}

output "daily_cleanup_task_queue" {
  value = data.temporal_schedule.example.action.workflow.task_queue
}
