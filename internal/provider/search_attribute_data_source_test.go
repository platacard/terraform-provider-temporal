package provider_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSearchAttributeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,

		Steps: []resource.TestStep{
			// Test reading from the server and setting data source attributes
			{
				Config: providerConfig + `
                data "temporal_search_attribute" "example" {
                    name = "RunId"
                    namespace = "default"
                }`,

				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "name", "RunId"),
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "type", "Keyword"), // Verify that the type was read correctly from the server
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "namespace", "default"),
				),
			},
		},
	})
}
