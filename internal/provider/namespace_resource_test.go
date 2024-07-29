package provider_test

import (
	"regexp"
	"testing"

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
