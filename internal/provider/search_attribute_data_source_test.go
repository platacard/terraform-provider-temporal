package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSearchAttributeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,

		Steps: []resource.TestStep{
			{
				Config: testAccSearchAttributeDataSourceConfig("default", "RunId"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "name", "RunId"),
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "type", "Keyword"),
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "namespace", "default"),
				),
			},
		},
	})
}

// Helper function to create Terraform configuration string
func testAccSearchAttributeDataSourceConfig(namespace, attributeName string) string {
	return providerConfig + `
data "temporal_search_attribute" "example" {
	name = "` + attributeName + `"
	namespace = "` + namespace + `"
}
`
}
