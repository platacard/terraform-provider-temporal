# A search attribute can be imported by specifying 'namespace:search_attibute_name'
terraform import temporal_search_attribute.example_search_attribute default:example


# A search attribute can also be imported by specifying just 'search_attibute_name' ('default' namespace will be used)
terraform import temporal_search_attribute.example_search_attribute example