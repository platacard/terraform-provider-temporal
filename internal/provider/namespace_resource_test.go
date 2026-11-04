package provider_test

import (
	"context"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNamespaceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "temporal_namespace" "test" {
	name        = "test"
	description = "This is a test namespace"
	owner_email = "test@example.org"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", "test"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "description", "This is a test namespace"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "test@example.org"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "is_global_namespace", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "temporal_namespace.test",
				ImportState:       true,
				ImportStateId:     "test",
				ImportStateVerify: false,
			},
			// Update and Read testing
			{
				Config: providerConfig + `
			resource "temporal_namespace" "test" {
				name        = "test"
				description = "This is a test namespace"
				owner_email = "updated@example.org"
			}
			`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", "test"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "updated@example.org"),
				),
			},
		},
	})
}

func TestAccNamespaceAlreadyExsits(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test1" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
				}`,
			},
			// Namespace already exists Error
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test2" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
				}
				`,
				ExpectError: regexp.MustCompile("namespace registration failed.*code = AlreadyExists"),
			},
		},
	})
}

func TestAccNamespaceSetReplicationConfig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "active"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", "test"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "active_cluster_name", "active"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "clusters.0", "active"),
				),
			},
		},
	})
}

func TestAccNamespaceSetReplicationWithChangeBothActiveClusterNameAndClusters(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "active"
					clusters = ["active"]
				}
				`,
			},
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "standby"
					clusters = ["standby"]
				}
				`,
				ExpectError: regexp.MustCompile("Cannot update active cluster name and clusters at the same time"),
			},
		},
	})
}

func TestAccNamespaceSetReplicationWithChangeActiveClusterNotInClusters(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "active"
					clusters = ["active"]
				}
				`,
			},
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "test"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "standby"
					clusters = ["active"]
				}
				`,
				ExpectError: regexp.MustCompile("Active cluster name is not in the clusters"),
			},
		},
	})
}

func TestAccNamespaceSubdayRetention(t *testing.T) {
	config := func(retention string) string {
		return providerConfig + `
resource "temporal_namespace" "subday" {
  name = "provider-test-subday"
  owner_email = "test@example.org"
  retention = 3
  retention_period = ` + retention + `
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("3"), Check: resource.TestCheckResourceAttr("temporal_namespace.subday", "retention", "3")},
			{Config: config("72h"), PlanOnly: true},
			{Config: config(`"1h1s"`), Check: resource.TestCheckResourceAttr("temporal_namespace.subday", "retention_period", "1h1s")},
			{ResourceName: "temporal_namespace.subday", ImportState: true, ImportStateId: "provider-test-subday", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"retention_period"}},

			{Config: config(`"90m30s"`), Check: resource.TestCheckResourceAttr("temporal_namespace.subday", "retention_period", "90m30s")},
			{Config: config(`"90m30s"`), PlanOnly: true},
		},
	})
}

// Start with an unmanaged namespace: persisting an import into an address that
// is already managed would fail before it exercised the post-import plan.
func TestAccNamespaceRetentionImportedPlan(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance test")
	}
	conn, err := grpc.NewClient("127.0.0.1:7233", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	const name = "provider-test-retention-import"
	_, err = workflowservice.NewWorkflowServiceClient(conn).RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace: name, OwnerEmail: "test@example.org",
		WorkflowExecutionRetentionPeriod: durationpb.New(time.Hour + time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = operatorservice.NewOperatorServiceClient(conn).DeleteNamespace(ctx, &operatorservice.DeleteNamespaceRequest{Namespace: name})
	}()
	const config = providerConfig + `
resource "temporal_namespace" "imported" {
  name = "provider-test-retention-import"
  owner_email = "test@example.org"
  retention_period = "1h1s"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, ResourceName: "temporal_namespace.imported", ImportState: true, ImportStateId: name, ImportStatePersist: true},
			{Config: config, PlanOnly: true},
		},
	})
}
