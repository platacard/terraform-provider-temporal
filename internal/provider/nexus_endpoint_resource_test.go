package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	nexusv1 "go.temporal.io/api/nexus/v1"
	"go.temporal.io/api/operatorservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func testAccNexusEndpointName() string {
	return "tf-acc-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
}

func testAccNexusOperatorClient() (operatorservice.OperatorServiceClient, func() error, error) {
	conn, err := grpc.NewClient("127.0.0.1:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}

	return operatorservice.NewOperatorServiceClient(conn), conn.Close, nil
}

func testAccCheckNexusEndpointDestroy(s *terraform.State) error {
	client, closeConn, err := testAccNexusOperatorClient()
	if err != nil {
		return err
	}
	defer func() { _ = closeConn() }()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "temporal_nexus_endpoint" {
			continue
		}
		_, err := client.GetNexusEndpoint(context.Background(), &operatorservice.GetNexusEndpointRequest{Id: rs.Primary.ID})
		if status.Code(err) != codes.NotFound {
			return fmt.Errorf("Nexus endpoint %s still exists (error: %v)", rs.Primary.ID, err)
		}
	}

	return nil
}

func testAccNexusEndpointWorkerConfig(name, description, taskQueue string) string {
	return providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  %s

  worker_target = {
    namespace  = "default"
    task_queue = %q
  }
}
`, name, description, taskQueue)
}

func testAccNexusEndpointExternalConfig(name, description, url string) string {
	return providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  name = %q
  %s

  external_target = {
    url = %q
  }
}
`, name, description, url)
}

func TestAccNexusEndpointResource_WorkerTarget(t *testing.T) {
	name := testAccNexusEndpointName()
	renamed := name + "-renamed"
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNexusEndpointWorkerConfig(name, `description = "first"`, "queue-one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
						id = value
						return nil
					}),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "description", "first"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.namespace", "default"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.task_queue", "queue-one"),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "external_target.url"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
					resource.TestMatchResourceAttr("temporal_nexus_endpoint.test", "url_prefix", regexp.MustCompile(`^/nexus/endpoints/[0-9a-f-]{36}/services$`)),
				),
			},
			{
				Config: testAccNexusEndpointWorkerConfig(name, `description = "second"`, "queue-two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "description", "second"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.task_queue", "queue-two"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "2"),
				),
			},
			{
				Config: testAccNexusEndpointWorkerConfig(renamed, "", "queue-two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
						if value != id {
							return fmt.Errorf("the endpoint was replaced on rename: id changed from %s to %s", id, value)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", renamed),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "description"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "3"),
				),
			},
			{
				ResourceName:      "temporal_nexus_endpoint.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "temporal_nexus_endpoint.test",
				ImportState:       true,
				ImportStateId:     renamed,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNexusEndpointResource_ExternalTarget(t *testing.T) {
	name := testAccNexusEndpointName()
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccNexusEndpointExternalConfig(name, `description = ""`, "https://example.com/nexus"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
						id = value
						return nil
					}),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "description", ""),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "external_target.url", "https://example.com/nexus"),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "worker_target.namespace"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
				),
			},
			{
				Config: testAccNexusEndpointExternalConfig(name, `description = ""`, "http://example.com/other"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "external_target.url", "http://example.com/other"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "2"),
				),
			},
			{
				Config: testAccNexusEndpointWorkerConfig(name, `description = ""`, "queue"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
						if value != id {
							return fmt.Errorf("the endpoint was replaced on target change: id changed from %s to %s", id, value)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "worker_target.task_queue", "queue"),
					resource.TestCheckNoResourceAttr("temporal_nexus_endpoint.test", "external_target.url"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "3"),
				),
			},
		},
	})
}

func TestAccNexusEndpointResource_ImportUpperCaseID(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test")
	}
	client, closeConn, err := testAccNexusOperatorClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := closeConn(); err != nil {
			t.Errorf("failed to close Temporal connection: %v", err)
		}
	}()
	ctx := context.Background()

	name := testAccNexusEndpointName()
	createResp, err := client.CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{
		Spec: &nexusv1.EndpointSpec{
			Name: name,
			Target: &nexusv1.EndpointTarget{Variant: &nexusv1.EndpointTarget_External_{
				External: &nexusv1.EndpointTarget_External{Url: "https://example.com/nexus"},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := createResp.GetEndpoint().GetId()
	defer func() {
		_, _ = client.DeleteNexusEndpoint(ctx, &operatorservice.DeleteNexusEndpointRequest{Id: id, Version: 1})
	}()

	config := testAccNexusEndpointExternalConfig(name, "", "https://example.com/nexus")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config:             config,
				ResourceName:       "temporal_nexus_endpoint.test",
				ImportState:        true,
				ImportStateId:      strings.ToUpper(id),
				ImportStatePersist: true,
			},
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "id", id),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
				),
			},
		},
	})

	if _, err := client.GetNexusEndpoint(ctx, &operatorservice.GetNexusEndpointRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Errorf("the imported endpoint %s was not deleted on destroy (error: %v)", id, err)
	}
}

func TestAccNexusEndpointResource_DeletedOutsideTerraform(t *testing.T) {
	name := testAccNexusEndpointName()
	config := testAccNexusEndpointWorkerConfig(name, "", "queue")
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
					id = value
					return nil
				}),
			},
			{
				PreConfig: func() {
					client, closeConn, err := testAccNexusOperatorClient()
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = closeConn() }()
					if _, err := client.DeleteNexusEndpoint(context.Background(), &operatorservice.DeleteNexusEndpointRequest{Id: id, Version: 1}); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrWith("temporal_nexus_endpoint.test", "id", func(value string) error {
						if value == id {
							return fmt.Errorf("expected a new endpoint, the id is still %s", id)
						}
						return nil
					}),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test", "version", "1"),
				),
			},
		},
	})
}

func TestAccNexusEndpointResource_Validation(t *testing.T) {
	cases := map[string]struct {
		config string
		err    string
	}{
		"underscore in name": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name            = "bad_name"
  external_target = { url = "https://example.com/nexus" }
}`,
			err: "Invalid Attribute Value Match",
		},
		"one-character name": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name            = "a"
  external_target = { url = "https://example.com/nexus" }
}`,
			err: "Invalid Attribute Value Match",
		},
		"unsupported URL scheme": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name            = "tf-acc-validation"
  external_target = { url = "ftp://example.com/nexus" }
}`,
			err: "Invalid Attribute Value Match",
		},
		"no target": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name = "tf-acc-validation"
}`,
			err: "Missing Attribute Configuration",
		},
		"both targets": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name            = "tf-acc-validation"
  worker_target   = { namespace = "default", task_queue = "queue" }
  external_target = { url = "https://example.com/nexus" }
}`,
			err: "Invalid Attribute Combination",
		},
		"empty task queue": {
			config: `
resource "temporal_nexus_endpoint" "test" {
  name          = "tf-acc-validation"
  worker_target = { namespace = "default", task_queue = "" }
}`,
			err: "Invalid Attribute Value Length",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig + c.config,
						PlanOnly:    true,
						ExpectError: regexp.MustCompile(c.err),
					},
				},
			})
		})
	}
}

func TestAccNexusEndpointResource_Multiple(t *testing.T) {
	prefix := testAccNexusEndpointName()
	config := func(taskQueue string) string {
		return providerConfig + fmt.Sprintf(`
resource "temporal_nexus_endpoint" "test" {
  count = 10
  name  = "%s-${count.index}"

  worker_target = {
    namespace  = "default"
    task_queue = %q
  }
}
`, prefix, taskQueue)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNexusEndpointDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("queue-one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test.0", "version", "1"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test.9", "version", "1"),
				),
			},
			{
				Config: config("queue-two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test.0", "worker_target.task_queue", "queue-two"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test.9", "worker_target.task_queue", "queue-two"),
					resource.TestCheckResourceAttr("temporal_nexus_endpoint.test.9", "version", "2"),
				),
			},
		},
	})
}
