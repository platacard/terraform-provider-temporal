package provider_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccScheduleResource_Basic(t *testing.T) {
	scheduleName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
  
  spec = {
    intervals = [{
      every = "24h"
      offset = "1h"
    }]
    time_zone = "America/New_York"
  }

  state = {
    paused = true
  }

  policy_config = {
    catchup_window = "10m"
    overlap_policy = "BufferOne"
  }
  
  action = {
    workflow = {
      workflow_id   = "test-workflow-1"
      workflow_type = "TestWorkflow"
      task_queue    = "test-queue"
      execution_timeout = "1h"
      run_timeout       = "30m"
      task_timeout      = "10s"
      input         = jsonencode({
        key    = "value"
        number = 123
      })
    }
  }
}
`, scheduleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_schedule.test", "schedule_id", scheduleName),
					resource.TestCheckResourceAttr("temporal_schedule.test", "namespace", "default"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.0.every", "24h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.0.offset", "1h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.time_zone", "America/New_York"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "state.paused", "true"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "policy_config.overlap_policy", "BufferOne"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.workflow_id", "test-workflow-1"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.workflow_type", "TestWorkflow"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.task_queue", "test-queue"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.input", `{"key":"value","number":123}`),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.execution_timeout", "1h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.run_timeout", "30m"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "action.workflow.task_timeout", "10s"),
				),
			},
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
  
  spec = {
    intervals = [{
      every = "12h"
    }]
    time_zone = "UTC"
  }

  state = {}

  policy_config = {
    catchup_window  = "5m"
    overlap_policy  = "Skip"
  }

  memo = {
    owner = "terraform"
  }

  action = {
    workflow = {
      workflow_id   = "test-workflow-1"
      workflow_type = "TestWorkflow"
      task_queue    = "test-queue"
      execution_timeout = "1h"
      run_timeout       = "30m"
      task_timeout      = "10s"
      input         = jsonencode({
        key    = "value"
        number = 123
      })
    }
  }
}
`, scheduleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.0.every", "12h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.time_zone", "UTC"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "policy_config.catchup_window", "5m"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "policy_config.overlap_policy", "Skip"),
				),
			},
			{
				ResourceName:      "temporal_schedule.test",
				ImportState:       true,
				ImportStateVerify: false,
				ImportStateId:     fmt.Sprintf("default:%s", scheduleName),
			},
		},
	})
}

func TestAccScheduleResource_MultipleIntervals(t *testing.T) {
	scheduleName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
  
  spec = {
    intervals = [
      {
        every = "12h"
      },
      {
        every  = "24h"
        offset = "6h"
      }
    ]
  }

  state = {}

  policy_config = {}

  action = {
    workflow = {
      workflow_id   = "test-workflow-7"
      workflow_type = "TestWorkflow"
      task_queue    = "test-queue"
    }
  }
}
`, scheduleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.#", "2"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.0.every", "12h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.1.every", "24h"),
					resource.TestCheckResourceAttr("temporal_schedule.test", "spec.intervals.1.offset", "6h"),
				),
			},
		},
	})
}

func TestAccScheduleResource_InvalidDuration(t *testing.T) {
	scheduleName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
  
  spec = {
    intervals = [{
      every = "invalid-duration"
    }]
  }

  state = {}

  policy_config = {}

  action = {
    workflow = {
      workflow_id   = "test-workflow-8"
      workflow_type = "TestWorkflow"
      task_queue    = "test-queue"
    }
  }
}
`, scheduleName),
				ExpectError: regexp.MustCompile("Invalid Interval Duration"),
			},
		},
	})
}

func TestAccScheduleResource_InvalidOverlapPolicy(t *testing.T) {
	scheduleName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_schedule" "bad" {
  namespace   = "default"
  schedule_id = "%s"

  spec = {
    intervals = [{ every = "1h" }]
  }
  state = {}
  policy_config = {
    overlap_policy = "Invalid"
  }

  action = {
    workflow = {
      workflow_id   = "wf-bad-policy"
      workflow_type = "TestWorkflow"
      task_queue    = "test-queue"
    }
  }
}
`, scheduleName),
				ExpectError: regexp.MustCompile(`(?i)overlap_policy`),
			},
		},
	})
}
