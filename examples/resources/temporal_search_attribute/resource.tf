# Manage an example search attribute, with or without explicitly providing a namespace
resource "temporal_search_attribute" "example_search_attribute" {
  name      = "example"
  type      = "Bool"
  namespace = "default"
}

resource "temporal_search_attribute" "example_search_attribute_no_namespace" {
  name = "example"
  type = "Bool"
  # 'default' namespace will be used here
}