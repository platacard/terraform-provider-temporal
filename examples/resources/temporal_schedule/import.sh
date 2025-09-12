# A schedule can be imported by specifying 'namespace:schedule_id'
terraform import temporal_schedule.example_schedule default:example


# A schedule can also be imported by specifying just 'schedule_id' ('default' namespace will be used)
terraform import temporal_schedule.example_schedule example
