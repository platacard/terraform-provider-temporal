# Import by server-generated endpoint ID (preferred — stable across renames).
terraform import temporal_nexus_endpoint.bilrost_notegen 425547bc-c673-4217-8d4e-581b853d5a01

# Or import by endpoint name (resolved via ListNexusEndpoints).
terraform import temporal_nexus_endpoint.bilrost_notegen bilrost-notegen
