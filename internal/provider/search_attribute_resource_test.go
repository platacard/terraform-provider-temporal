package provider_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Replace these to test importing a custom search attribute created outside of terrafrom
const (
	importedAttributeName      = "testAttr"
	importedAttributeType      = "Keyword"
	importedAttributeNamespace = "default"
)

func TestAccSearchAttributeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{

			// Test creating and reading a search attribute
			{
				Config: providerConfig + `
				resource "temporal_search_attribute" "test" {
					namespace = "default"
					name = "testAttr"
					type = "Keyword"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "name", "testAttr"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "type", "Keyword"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "namespace", "default"),
				),
			},

			// Explicitly test read by using PlanOnly, which will call Read() and generate a plan without applying it
			// Since our config matches the server state, the plan should be empty and won't error
			{
				Config: providerConfig + `
				resource "temporal_search_attribute" "test" {
					namespace = "default"
					name = "testAttr"
					type = "Keyword"
				}`,
				PlanOnly: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "name", "testAttr"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "type", "Keyword"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "namespace", "default"),
				),
			},

			// Test importing an existing search attribute
			/*
				NOTE: This test can be used to test importing a search attribute created outside of terrafrom. To do this,
				use the Temporal CLI to create a custom search attribute and replace the importedAttribute constants to match
				your created attribute
			*/
			{
				Config: providerConfig + `resource "temporal_search_attribute" "test_0" {}`,

				ResourceName:     "temporal_search_attribute.test_0",
				ImportState:      true,
				ImportStateId:    fmt.Sprintf("%s:%s", importedAttributeNamespace, importedAttributeName), // 'namespace:attributeName'
				ImportStateCheck: checkImportedResourceAttributes,                                         // Verifies resource attributes post-import
			},

			// Test destorying all existing resources
			{
				Config: providerConfig,
				Check:  testAccCheckExampleResourceDestroy, // Verifies that there are no remaining search attribute resources
			},
		},
	})
}

// Verifies the attributes of a resource post-import
func checkImportedResourceAttributes(states []*terraform.InstanceState) error {
	if len(states) == 0 {
		return fmt.Errorf("no instances are available for import check")
	}
	state := states[0] // First instance is usually the primary instance in Terraform.

	if state.Attributes["name"] != importedAttributeName {
		return fmt.Errorf("incorrect name attribute; expected '%s', got '%s'", importedAttributeName, state.Attributes["name"])
	}
	if state.Attributes["type"] != importedAttributeType {
		return fmt.Errorf("incorrect type attribute; expected '%s', got '%s'", importedAttributeType, state.Attributes["type"])
	}
	if state.Attributes["namespace"] != importedAttributeNamespace {
		return fmt.Errorf("incorrect namespace attribute; expected '%s', got '%s'", importedAttributeNamespace, state.Attributes["namespace"])
	}

	return nil
}

// Verifies that all temporal_search_attribute resources have been successfully destroyed
func testAccCheckExampleResourceDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type == "temporal_search_attribute" {
			return fmt.Errorf("Found undeleted resource, attribute %s still exists", rs.Primary.Attributes["name"])
		}
	}
	return nil
}
