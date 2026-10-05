# Import by server-generated endpoint ID (preferred — stable identifier).
terraform import temporal_nexus_endpoint.order_processing 11111111-2222-3333-4444-555555555555

# Or import by endpoint name (resolved via ListNexusEndpoints).
terraform import temporal_nexus_endpoint.order_processing order-processing
