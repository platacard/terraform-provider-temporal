package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	commonv1 "go.temporal.io/api/common/v1"
	schedulev1 "go.temporal.io/api/schedule/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	workflowv1 "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testAccScheduleDataSourceListSize(attribute string, size int) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(
		"data.temporal_schedule.test",
		tfjsonpath.New("spec").AtMapKey(attribute),
		knownvalue.ListSizeExact(size),
	)
}

func TestAccScheduleDataSource(t *testing.T) {
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
      every  = "48h"
      offset = "1h"
    }]
    jitter    = "5m"
    time_zone = "Europe/Berlin"
  }

  state = {
    paused = true
    notes  = "paused by test"
  }

  policy_config = {
    catchup_window   = "10m"
    overlap_policy   = "BufferOne"
    pause_on_failure = true
  }

  memo = {
    owner = "terraform"
  }

  action = {
    workflow = {
      workflow_id       = "test-workflow-ds"
      workflow_type     = "TestWorkflow"
      task_queue        = "test-queue"
      execution_timeout = "1h"
      run_timeout       = "30m"
      task_timeout      = "10s"
      input = jsonencode({
        key = "value"
      })
    }
  }
}

data "temporal_schedule" "test" {
  namespace   = temporal_schedule.test.namespace
  schedule_id = temporal_schedule.test.schedule_id
}
`, scheduleName),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccScheduleDataSourceListSize("calendar_items", 0),
					testAccScheduleDataSourceListSize("cron_items", 0),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "namespace", "default"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "schedule_id", scheduleName),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.%", "1"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.owner", "terraform"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.intervals.#", "1"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.intervals.0.every", "48h"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.intervals.0.offset", "1h"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.jitter", "5m"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.time_zone", "Europe/Berlin"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.paused", "true"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.notes", "paused by test"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.limited_actions", "false"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.remaining_actions", "0"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.overlap_policy", "BufferOne"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.catchup_window", "10m"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.pause_on_failure", "true"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.workflow_id", "test-workflow-ds"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.workflow_type", "TestWorkflow"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.task_queue", "test-queue"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.input", `{"key":"value"}`),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.execution_timeout", "1h"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.run_timeout", "30m"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.task_timeout", "10s"),
				),
			},
		},
	})
}

func TestAccScheduleDataSource_CreatedOutsideTerraform(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test")
	}
	conn, err := grpc.NewClient("127.0.0.1:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("failed to close Temporal connection: %v", err)
		}
	}()
	ctx := context.Background()
	client := workflowservice.NewWorkflowServiceClient(conn)

	scheduleName := "provider-test-ds-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	requestID, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	jsonPayload := func(data string) *commonv1.Payload {
		return &commonv1.Payload{
			Metadata: map[string][]byte{"encoding": []byte("json/plain")},
			Data:     []byte(data),
		}
	}
	_, err = client.CreateSchedule(ctx, &workflowservice.CreateScheduleRequest{
		Namespace:  "default",
		ScheduleId: scheduleName,
		RequestId:  requestID,
		Memo: &commonv1.Memo{Fields: map[string]*commonv1.Payload{
			"text":   jsonPayload(`"plain"`),
			"number": jsonPayload(`42`),
			"object": jsonPayload(`{"a":1}`),
			"binary": {Data: []byte{0xff, 0xfe}},
		}},
		Schedule: &schedulev1.Schedule{
			Spec: &schedulev1.ScheduleSpec{
				CronString: []string{"15 8 * * MON-FRI"},
				Interval:   []*schedulev1.IntervalSpec{{Interval: durationpb.New(48 * time.Hour)}},
				StartTime:  timestamppb.New(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)),
				EndTime:    timestamppb.New(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)),
			},
			Action: &schedulev1.ScheduleAction{Action: &schedulev1.ScheduleAction_StartWorkflow{
				StartWorkflow: &workflowv1.NewWorkflowExecutionInfo{
					WorkflowId:   "test-workflow-ds-external",
					WorkflowType: &commonv1.WorkflowType{Name: "TestWorkflow"},
					TaskQueue:    &taskqueuev1.TaskQueue{Name: "test-queue"},
					Input:        &commonv1.Payloads{Payloads: []*commonv1.Payload{{Data: []byte{0xff, 0xfe}}}},
				},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.DeleteSchedule(ctx, &workflowservice.DeleteScheduleRequest{Namespace: "default", ScheduleId: scheduleName})
	}()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
}
`, scheduleName),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccScheduleDataSourceListSize("cron_items", 0),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.%", "4"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.text", "plain"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.number", "42"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.object", `{"a":1}`),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "memo.binary", "//4="),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.intervals.#", "1"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.intervals.0.every", "48h"),
					resource.TestCheckNoResourceAttr("data.temporal_schedule.test", "spec.intervals.0.offset"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.#", "1"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.second", "0"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.minute", "15"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.hour", "8"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.day_of_month", "1-31"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.month", "1-12"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.day_of_week", "1-5"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.year", ""),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.start_time", "2030-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.end_time", "2031-01-01T00:00:00Z"),
					resource.TestCheckNoResourceAttr("data.temporal_schedule.test", "spec.jitter"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.time_zone", ""),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.paused", "false"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "state.notes", ""),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.overlap_policy", "Skip"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.catchup_window", "8760h"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "policy_config.pause_on_failure", "false"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.workflow_id", "test-workflow-ds-external"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.workflow_type", "TestWorkflow"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.task_queue", "test-queue"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.input", "//4="),
					resource.TestCheckNoResourceAttr("data.temporal_schedule.test", "action.workflow.execution_timeout"),
				),
			},
		},
	})
}

func TestAccScheduleDataSource_CronOnly(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test")
	}
	conn, err := grpc.NewClient("127.0.0.1:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("failed to close Temporal connection: %v", err)
		}
	}()
	ctx := context.Background()
	client := workflowservice.NewWorkflowServiceClient(conn)

	scheduleName := "provider-test-ds-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	requestID, err := uuid.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateSchedule(ctx, &workflowservice.CreateScheduleRequest{
		Namespace:  "default",
		ScheduleId: scheduleName,
		RequestId:  requestID,
		Schedule: &schedulev1.Schedule{
			Spec: &schedulev1.ScheduleSpec{CronString: []string{"0 12 * * *"}},
			Action: &schedulev1.ScheduleAction{Action: &schedulev1.ScheduleAction_StartWorkflow{
				StartWorkflow: &workflowv1.NewWorkflowExecutionInfo{
					WorkflowId:   "test-workflow-ds-cron",
					WorkflowType: &commonv1.WorkflowType{Name: "TestWorkflow"},
					TaskQueue:    &taskqueuev1.TaskQueue{Name: "test-queue"},
					Input: &commonv1.Payloads{Payloads: []*commonv1.Payload{
						{Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte(`"acct-1"`)},
						{Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte(`42`)},
					}},
				},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.DeleteSchedule(ctx, &workflowservice.DeleteScheduleRequest{Namespace: "default", ScheduleId: scheduleName})
	}()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "temporal_schedule" "test" {
  namespace   = "default"
  schedule_id = "%s"
}
`, scheduleName),
				ConfigStateChecks: []statecheck.StateCheck{
					testAccScheduleDataSourceListSize("intervals", 0),
					testAccScheduleDataSourceListSize("calendar_items", 1),
					testAccScheduleDataSourceListSize("cron_items", 0),
					statecheck.ExpectKnownValue("data.temporal_schedule.test", tfjsonpath.New("memo"), knownvalue.MapSizeExact(0)),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.hour", "12"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "spec.calendar_items.0.minute", "0"),
					resource.TestCheckResourceAttr("data.temporal_schedule.test", "action.workflow.input", `"acct-1"`),
				),
			},
		},
	})
}

func TestAccScheduleDataSource_NotFound(t *testing.T) {
	scheduleName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "temporal_schedule" "missing" {
  namespace   = "default"
  schedule_id = "%s"
}
`, scheduleName),
				ExpectError: regexp.MustCompile("Could not read schedule"),
			},
		},
	})
}
