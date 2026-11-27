package provider_test

import (
	"context"
	"fmt"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccNamespaceResource(t *testing.T) {
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
resource "temporal_namespace" "test" {
	name        = "` + nsName + `"
	description = "This is a test namespace"
	owner_email = "test@example.org"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", nsName),
					resource.TestCheckResourceAttr("temporal_namespace.test", "description", "This is a test namespace"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "test@example.org"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "is_global_namespace", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "temporal_namespace.test",
				ImportState:       true,
				ImportStateId:     nsName,
				ImportStateVerify: false,
			},
			// Update and Read testing
			{
				Config: providerConfig + `
			resource "temporal_namespace" "test" {
				name        = "` + nsName + `"
				description = "This is a test namespace"
				owner_email = "updated@example.org"
			}
			`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", nsName),
					resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "updated@example.org"),
				),
			},
		},
	})
}

func TestAccNamespaceWithoutOwnerEmail(t *testing.T) {
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without owner_email
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", nsName),
					resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", ""),
				),
			},
			// Set owner_email on an existing namespace
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
					owner_email = "test@example.org"
				}
				`,
				Check: resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "test@example.org"),
			},
			// Removing owner_email from the config keeps the existing value
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
				}
				`,
				Check: resource.TestCheckResourceAttr("temporal_namespace.test", "owner_email", "test@example.org"),
			},
		},
	})
}

func TestAccNamespaceAlreadyExsits(t *testing.T) {
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test1" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
					owner_email = "test@example.org"
				}`,
			},
			// Namespace already exists Error
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test1" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
					owner_email = "test@example.org"
				}

				resource "temporal_namespace" "test2" {
					name        = "` + nsName + `"
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
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
					description = "This is a test namespace"
					owner_email = "test@example.org"
					active_cluster_name = "active"
				}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_namespace.test", "name", nsName),
					resource.TestCheckResourceAttr("temporal_namespace.test", "active_cluster_name", "active"),
					resource.TestCheckResourceAttr("temporal_namespace.test", "clusters.0", "active"),
				),
			},
		},
	})
}

func TestAccNamespaceSetReplicationWithChangeBothActiveClusterNameAndClusters(t *testing.T) {
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
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
					name        = "` + nsName + `"
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
	nsName := acctest.RandomWithPrefix("tf-acc-test")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
				resource "temporal_namespace" "test" {
					name        = "` + nsName + `"
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
					name        = "` + nsName + `"
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
	nsName := acctest.RandomWithPrefix("tf-acc-test")
	config := func(retention string) string {
		return providerConfig + `
resource "temporal_namespace" "subday" {
  name = "` + nsName + `"
  owner_email = "test@example.org"
  retention_period = ` + retention + `
}
`
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(`"72h"`), Check: resource.TestCheckResourceAttr("temporal_namespace.subday", "retention", "3")},
			{Config: config(`"72h"`), PlanOnly: true},
			{Config: config(`"1h1s"`), Check: resource.TestCheckResourceAttr("temporal_namespace.subday", "retention_period", "1h1s")},
			{ResourceName: "temporal_namespace.subday", ImportState: true, ImportStateId: nsName, ImportStateVerify: true, // Import returns canonical duration spelling.
				ImportStateVerifyIgnore: []string{"retention_period"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported namespace, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["retention"] != "0" || attrs["retention_period"] != "1h0m1s" {
						return fmt.Errorf("unexpected imported retention: days=%q, period=%q", attrs["retention"], attrs["retention_period"])
					}
					return nil
				},
			},

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
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("failed to close Temporal connection: %v", err)
		}
	}()
	ctx := context.Background()
	name := acctest.RandomWithPrefix("tf-acc-test")
	_, err = workflowservice.NewWorkflowServiceClient(conn).RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace: name, OwnerEmail: "test@example.org",
		WorkflowExecutionRetentionPeriod: durationpb.New(time.Hour + time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_, _ = operatorservice.NewOperatorServiceClient(conn).DeleteNamespace(cleanupCtx, &operatorservice.DeleteNamespaceRequest{Namespace: name})
	}()
	config := providerConfig + `
resource "temporal_namespace" "imported" {
  name = "` + name + `"
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

func TestAccNamespaceRetentionConflict(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig + `
resource "temporal_namespace" "conflict" {
  name = "provider-test-retention-conflict"
  owner_email = "test@example.org"
  retention = 3
  retention_period = "1h"
}
`,
			ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
		}},
	})
}

func TestAccNamespaceLegacyRetentionUpdates(t *testing.T) {
	nsName := acctest.RandomWithPrefix("tf-acc-test")
	config := func(days int) string {
		return providerConfig + fmt.Sprintf(`
resource "temporal_namespace" "legacy" {
  name = %q
  owner_email = "test@example.org"
  retention = %d
}
`, nsName, days)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(3), Check: resource.TestCheckResourceAttr("temporal_namespace.legacy", "retention_period", "72h0m0s")},
			{Config: config(2), Check: resource.TestCheckResourceAttr("temporal_namespace.legacy", "retention_period", "48h0m0s")},
			{Config: config(2), PlanOnly: true},
		},
	})
}

func TestAccNamespaceRetentionDefaultOnRemoval(t *testing.T) {
	for _, attribute := range []string{"retention = 7", "retention_period = \"168h\""} {
		t.Run(attribute, func(t *testing.T) {
			nsName := acctest.RandomWithPrefix("tf-acc-test")
			config := func(retention, description string) string {
				return providerConfig + fmt.Sprintf(`
resource "temporal_namespace" "defaulting" {
  name = %q
  owner_email = "test@example.org"
  description = %q
  %s
}
`, nsName, description, retention)
			}
			plannedDefault := resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
				plancheck.ExpectKnownValue("temporal_namespace.defaulting", tfjsonpath.New("retention"), knownvalue.Int64Exact(3)),
				plancheck.ExpectKnownValue("temporal_namespace.defaulting", tfjsonpath.New("retention_period"), knownvalue.StringExact("72h0m0s")),
			}}
			stateDefault := resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("temporal_namespace.defaulting", "retention", "3"),
				resource.TestCheckResourceAttr("temporal_namespace.defaulting", "retention_period", "72h0m0s"),
			)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config(attribute, "original"), Check: resource.TestCheckResourceAttr("temporal_namespace.defaulting", "retention", "7")},
					{Config: config("", "original"), PlanOnly: true, ExpectNonEmptyPlan: true},
					{Config: config("", "original"), ConfigPlanChecks: plannedDefault, Check: stateDefault},
					{Config: config("", "changed"), ConfigPlanChecks: plannedDefault, Check: stateDefault},
					{Config: config("", "changed"), PlanOnly: true},
				},
			})
		})
	}
}
