# Get data of the example search attribute, with or without explicitly providing a namespace
data "temporal_search_attribute" "example_search_attribute" {
  name      = "example"
  namespace = "default"
}

data "temporal_search_attribute" "example_search_attribute_no_namespace" {
  name = "example"
  # 'default' namespace will be used here
}
