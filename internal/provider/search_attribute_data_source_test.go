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

			// Test reading data source without explicitly proving namespace, should use 'default'
			{
				Config: providerConfig + `
                data "temporal_search_attribute" "example" {
                    name = "RunId"
                }`,

				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "name", "RunId"),
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "type", "Keyword"), // Verify that the type was read correctly from the server
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "namespace", "default"),
				),
			},

			// Test reading with provided namespace
			{
				Config: providerConfig + `
                data "temporal_search_attribute" "example" {
                    name = "HistoryLength"
					namespace = "default"
                }`,

				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "name", "HistoryLength"),
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "type", "Int"), // Verify that the type was read correctly from the server
					resource.TestCheckResourceAttr("data.temporal_search_attribute.example", "namespace", "default"),
				),
			},
		},
	})
}
