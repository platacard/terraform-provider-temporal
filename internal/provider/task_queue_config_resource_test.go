package provider_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"go.temporal.io/api/enums/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func testAccTaskQueueName() string {
	return "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
}

func testAccWorkflowClient() (workflowservice.WorkflowServiceClient, func() error, error) {
	conn, err := grpc.NewClient("127.0.0.1:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}

	return workflowservice.NewWorkflowServiceClient(conn), conn.Close, nil
}

func testAccTaskQueueConfig(taskQueue, taskQueueType string) (*taskqueuev1.TaskQueueConfig, error) {
	client, closeConn, err := testAccWorkflowClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeConn() }()

	parsedType, err := enums.TaskQueueTypeFromString(taskQueueType)
	if err != nil {
		return nil, err
	}
	describeResp, err := client.DescribeTaskQueue(context.Background(), &workflowservice.DescribeTaskQueueRequest{
		Namespace:     "default",
		TaskQueue:     &taskqueuev1.TaskQueue{Name: taskQueue},
		TaskQueueType: parsedType,
		ReportConfig:  true,
	})
	if err != nil {
		return nil, err
	}

	return describeResp.GetConfig(), nil
}

func testAccUpdateTaskQueueConfig(t *testing.T, request *workflowservice.UpdateTaskQueueConfigRequest) {
	client, closeConn, err := testAccWorkflowClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeConn() }()

	request.Namespace = "default"
	if _, err := client.UpdateTaskQueueConfig(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func testAccTaskQueueConfigSupported(t *testing.T) bool {
	client, closeConn, err := testAccWorkflowClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeConn() }()

	_, err = client.UpdateTaskQueueConfig(context.Background(), &workflowservice.UpdateTaskQueueConfigRequest{
		Namespace:     "default",
		TaskQueue:     "tf-acc-precheck",
		TaskQueueType: enums.TASK_QUEUE_TYPE_ACTIVITY,
	})
	if status.Code(err) == codes.Unimplemented {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}

	return true
}

func testAccPreCheckTaskQueueConfig(t *testing.T) {
	if !testAccTaskQueueConfigSupported(t) {
		t.Skip("the Temporal server does not support UpdateTaskQueueConfig, which requires Temporal Server 1.29 or later")
	}
}

func testAccCheckTaskQueueConfigDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "temporal_task_queue_config" {
			continue
		}
		taskQueue := rs.Primary.Attributes["task_queue"]
		taskQueueType := rs.Primary.Attributes["task_queue_type"]
		config, err := testAccTaskQueueConfig(taskQueue, taskQueueType)
		if err != nil {
			return err
		}
		if config.GetQueueRateLimit().GetRateLimit() != nil || config.GetFairnessKeysRateLimitDefault().GetRateLimit() != nil || len(config.GetFairnessWeightOverrides()) != 0 {
			return fmt.Errorf("configuration of %s task queue %s was not reset: %v", taskQueueType, taskQueue, config)
		}
	}

	return nil
}

func testAccTaskQueueConfigResource(taskQueue, taskQueueType, body string) string {
	return providerConfig + fmt.Sprintf(`
resource "temporal_task_queue_config" "test" {
  namespace       = "default"
  task_queue      = %q
  task_queue_type = %q
%s
}
`, taskQueue, taskQueueType, body)
}

func TestAccTaskQueueConfigResource_Activity(t *testing.T) {
	taskQueue := testAccTaskQueueName()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTaskQueueConfigResource(taskQueue, "Activity", `
  queue_rate_limit = {
    requests_per_second = 25
    reason              = "protect downstream"
  }

  fairness_key_rate_limit_default = {
    requests_per_second = 5
  }

  fairness_weight_overrides = {
    "tenant-a" = 3.5
    "tenant-b" = 0.5
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "namespace", "default"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "task_queue", taskQueue),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "task_queue_type", "Activity"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.requests_per_second", "25"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.reason", "protect downstream"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_key_rate_limit_default.requests_per_second", "5"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_key_rate_limit_default.reason", ""),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.%", "2"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-a", "3.5"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-b", "0.5"),
					func(*terraform.State) error {
						config, err := testAccTaskQueueConfig(taskQueue, "Activity")
						if err != nil {
							return err
						}
						if got := config.GetQueueRateLimit().GetRateLimit().GetRequestsPerSecond(); got != 25 {
							return fmt.Errorf("queue rate limit on the server: got %v, want 25", got)
						}
						if got := config.GetFairnessWeightOverrides()["tenant-a"]; got != 3.5 {
							return fmt.Errorf("weight of tenant-a on the server: got %v, want 3.5", got)
						}
						return nil
					},
				),
			},
			{
				Config: testAccTaskQueueConfigResource(taskQueue, "Activity", `
  queue_rate_limit = {
    requests_per_second = 0
    reason              = "paused"
  }

  fairness_weight_overrides = {
    "tenant-a" = 2
    "tenant-c" = 10
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.requests_per_second", "0"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.reason", "paused"),
					resource.TestCheckNoResourceAttr("temporal_task_queue_config.test", "fairness_key_rate_limit_default.requests_per_second"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.%", "2"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-a", "2"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-c", "10"),
					func(*terraform.State) error {
						config, err := testAccTaskQueueConfig(taskQueue, "Activity")
						if err != nil {
							return err
						}
						if config.GetQueueRateLimit().GetRateLimit() == nil {
							return fmt.Errorf("a zero queue rate limit must stay set on the server")
						}
						if config.GetFairnessKeysRateLimitDefault().GetRateLimit() != nil {
							return fmt.Errorf("the fairness key rate limit was not removed from the server")
						}
						if _, ok := config.GetFairnessWeightOverrides()["tenant-b"]; ok {
							return fmt.Errorf("the weight of tenant-b was not removed from the server")
						}
						return nil
					},
				),
			},
			{
				ResourceName:                         "temporal_task_queue_config.test",
				ImportState:                          true,
				ImportStateId:                        fmt.Sprintf("default:%s:Activity", taskQueue),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "task_queue",
			},
			{
				Config: testAccTaskQueueConfigResource(taskQueue, "Activity", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.requests_per_second"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.%", "0"),
				),
			},
		},
	})
}

func TestAccTaskQueueConfigResource_FractionalValues(t *testing.T) {
	taskQueue := testAccTaskQueueName()
	config := testAccTaskQueueConfigResource(taskQueue, "Nexus", `
  queue_rate_limit = {
    requests_per_second = 0.1
  }

  fairness_key_rate_limit_default = {
    requests_per_second = 0.3
  }

  fairness_weight_overrides = {
    "tenant-a" = 0.7
    "tenant-b" = 0.001
    "tenant-c" = 1000
  }
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.requests_per_second", "0.1"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_key_rate_limit_default.requests_per_second", "0.3"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-a", "0.7"),
				),
			},
			{
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "queue_rate_limit.requests_per_second", "0.1"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-a", "0.7"),
				),
			},
		},
	})
}

func TestAccTaskQueueConfigResource_ImportFractionalValues(t *testing.T) {
	taskQueue := testAccTaskQueueName()
	config := testAccTaskQueueConfigResource(taskQueue, "Activity", `
  queue_rate_limit = {
    requests_per_second = 0.1
    reason              = "set outside Terraform"
  }

  fairness_weight_overrides = {
    "tenant-a" = 0.7
  }
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					testAccUpdateTaskQueueConfig(t, &workflowservice.UpdateTaskQueueConfigRequest{
						TaskQueue:     taskQueue,
						TaskQueueType: enums.TASK_QUEUE_TYPE_ACTIVITY,
						UpdateQueueRateLimit: &workflowservice.UpdateTaskQueueConfigRequest_RateLimitUpdate{
							RateLimit: &taskqueuev1.RateLimit{RequestsPerSecond: 0.1},
							Reason:    "set outside Terraform",
						},
						SetFairnessWeightOverrides: map[string]float32{"tenant-a": 0.7},
					})
				},
				Config:             config,
				ResourceName:       "temporal_task_queue_config.test",
				ImportState:        true,
				ImportStateId:      fmt.Sprintf("default:%s:Activity", taskQueue),
				ImportStatePersist: true,
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

func TestAccTaskQueueConfigResource_Workflow(t *testing.T) {
	taskQueue := testAccTaskQueueName()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccTaskQueueConfigResource(taskQueue, "Workflow", `
  fairness_weight_overrides = {
    "tenant-a" = 4
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "task_queue_type", "Workflow"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.test", "fairness_weight_overrides.tenant-a", "4"),
				),
			},
		},
	})
}

func TestAccTaskQueueConfigResource_TakesOverExistingConfiguration(t *testing.T) {
	taskQueue := testAccTaskQueueName()
	config := testAccTaskQueueConfigResource(taskQueue, "Activity", `
  fairness_weight_overrides = {
    "tenant-a" = 2
  }
`)
	checkServer := func(*terraform.State) error {
		got, err := testAccTaskQueueConfig(taskQueue, "Activity")
		if err != nil {
			return err
		}
		if got.GetQueueRateLimit().GetRateLimit() != nil {
			return fmt.Errorf("the queue rate limit set outside Terraform was not removed: %v", got)
		}
		if len(got.GetFairnessWeightOverrides()) != 1 || got.GetFairnessWeightOverrides()["tenant-a"] != 2 {
			return fmt.Errorf("unexpected fairness weights on the server: %v", got.GetFairnessWeightOverrides())
		}
		return nil
	}
	setOutsideTerraform := func() {
		testAccUpdateTaskQueueConfig(t, &workflowservice.UpdateTaskQueueConfigRequest{
			TaskQueue:     taskQueue,
			TaskQueueType: enums.TASK_QUEUE_TYPE_ACTIVITY,
			UpdateQueueRateLimit: &workflowservice.UpdateTaskQueueConfigRequest_RateLimitUpdate{
				RateLimit: &taskqueuev1.RateLimit{RequestsPerSecond: 7},
			},
			SetFairnessWeightOverrides: map[string]float32{"tenant-a": 9, "tenant-x": 3},
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				PreConfig: setOutsideTerraform,
				Config:    config,
				Check:     checkServer,
			},
			{
				PreConfig: setOutsideTerraform,
				Config:    config,
				Check:     checkServer,
			},
		},
	})
}

func TestAccTaskQueueConfigResource_Multiple(t *testing.T) {
	prefix := testAccTaskQueueName()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckTaskQueueConfig(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTaskQueueConfigDestroy,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "temporal_task_queue_config" "activity" {
  count           = 5
  namespace       = "default"
  task_queue      = "%[1]s-${count.index}"
  task_queue_type = "Activity"

  queue_rate_limit = {
    requests_per_second = 10 + count.index
  }
}

resource "temporal_task_queue_config" "nexus" {
  count           = 5
  namespace       = "default"
  task_queue      = "%[1]s-${count.index}"
  task_queue_type = "Nexus"

  queue_rate_limit = {
    requests_per_second = 20 + count.index
  }
}

resource "temporal_task_queue_config" "workflow" {
  count           = 5
  namespace       = "default"
  task_queue      = "%[1]s-${count.index}"
  task_queue_type = "Workflow"

  fairness_weight_overrides = {
    "tenant-a" = 1 + count.index
  }
}
`, prefix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_task_queue_config.activity.4", "queue_rate_limit.requests_per_second", "14"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.nexus.4", "queue_rate_limit.requests_per_second", "24"),
					resource.TestCheckResourceAttr("temporal_task_queue_config.workflow.4", "fairness_weight_overrides.tenant-a", "5"),
				),
			},
		},
	})
}

func TestAccTaskQueueConfigResource_UnsupportedServer(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			if testAccTaskQueueConfigSupported(t) {
				t.Skip("the Temporal server supports UpdateTaskQueueConfig")
			}
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTaskQueueConfigResource(testAccTaskQueueName(), "Activity", `
  queue_rate_limit = {
    requests_per_second = 1
  }
`),
				ExpectError: regexp.MustCompile(`requires\s+Temporal\s+Server\s+1\.29`),
			},
		},
	})
}

func TestAccTaskQueueConfigResource_Validation(t *testing.T) {
	cases := map[string]struct {
		taskQueueType string
		body          string
		err           string
	}{
		"negative rate limit": {
			taskQueueType: "Activity",
			body:          `queue_rate_limit = { requests_per_second = -1 }`,
			err:           "Invalid Attribute Value",
		},
		"weight below the minimum": {
			taskQueueType: "Activity",
			body:          `fairness_weight_overrides = { "tenant-a" = 0 }`,
			err:           "Invalid Attribute Value",
		},
		"weight above the maximum": {
			taskQueueType: "Activity",
			body:          `fairness_weight_overrides = { "tenant-a" = 1001 }`,
			err:           "Invalid Attribute Value",
		},
		"rate limit with more precision than the server stores": {
			taskQueueType: "Activity",
			body:          `queue_rate_limit = { requests_per_second = 0.123456789 }`,
			err:           "Invalid Attribute Value Precision",
		},
		"weight with more precision than the server stores": {
			taskQueueType: "Activity",
			body:          `fairness_weight_overrides = { "tenant-a" = 1.23456789 }`,
			err:           "Invalid Attribute Value Precision",
		},
		"unknown task queue type": {
			taskQueueType: "Sticky",
			err:           "Invalid Attribute Value Match",
		},
		"queue rate limit on a Workflow task queue": {
			taskQueueType: "Workflow",
			body:          `queue_rate_limit = { requests_per_second = 1 }`,
			err:           "Invalid Attribute Combination",
		},
		"fairness key rate limit on a Workflow task queue": {
			taskQueueType: "Workflow",
			body:          `fairness_key_rate_limit_default = { requests_per_second = 1 }`,
			err:           "Invalid Attribute Combination",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      testAccTaskQueueConfigResource("tf-acc-validation", c.taskQueueType, c.body),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(c.err),
					},
				},
			})
		})
	}
}
