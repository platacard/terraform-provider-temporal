package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
