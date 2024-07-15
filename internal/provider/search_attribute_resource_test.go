package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSearchAttributeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Test creating and reading a search attribute
			{
				Config: testAccSearchAttributeConfig("default", "test_attr", "Keyword"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "name", "test_attr"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "type", "Keyword"),
					resource.TestCheckResourceAttr("temporal_search_attribute.test", "namespace", "default"),
				),
			},
			// Test importing an existing search attribute
			{
				ResourceName:      "temporal_search_attribute.test",
				ImportState:       true,
				ImportStateId:     "default:test_attr",
				ImportStateVerify: false,
			},
		},
	})
}

// Helper function to create Terraform configuration string
func testAccSearchAttributeConfig(namespace, name, attrType string) string {
	return providerConfig + `
resource "temporal_search_attribute" "test" {
	namespace = "` + namespace + `"
	name = "` + name + `"
	type = "` + attrType + `"
}
`
}
