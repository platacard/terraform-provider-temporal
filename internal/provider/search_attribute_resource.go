package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jpillora/maplock"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc"
)

var (
	_ resource.Resource                = &SearchAttributeResource{}
	_ resource.ResourceWithConfigure   = &SearchAttributeResource{}
	_ resource.ResourceWithImportState = &SearchAttributeResource{}

	// namespaceLocks serializes search attribute mutations per namespace.
	// Concurrent AddSearchAttributes calls to the same namespace return success
	// but only the last write persists — the others are silently lost.
	namespaceLocks = maplock.New()
)

const (
	// searchAttributeAwaitTimeout is the maximum time to wait for a search
	// attribute to appear after calling AddSearchAttributes.
	searchAttributeAwaitTimeout = 1 * time.Minute
)

// awaitSearchAttribute polls ListSearchAttributes until the named attribute
// appears in CustomAttributes, or the timeout expires.
func awaitSearchAttribute(ctx context.Context, client operatorservice.OperatorServiceClient, data SearchAttributeResourceModel, timeout time.Duration) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			attributes, err := client.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
				Namespace: data.Namespace.ValueString(),
			})
			if err != nil {
				return err
			}
			if _, ok := attributes.CustomAttributes[data.Name.ValueString()]; ok {
				return nil
			}
		case <-deadline:
			return fmt.Errorf("timed out waiting for search attribute %q to appear after %s", data.Name.ValueString(), timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// withNamespaceLock serializes operations on a given namespace.
func withNamespaceLock(ns string, f func()) {
	namespaceLocks.Lock(ns)
	defer func() { _ = namespaceLocks.Unlock(ns) }()
	f()
}

// NewSearchAttributeResource creates a new instance of SearchAttributeResource.
func NewSearchAttributeResource() resource.Resource {
	return &SearchAttributeResource{}
}

// SearchAttributeResource - a Temporal search attribute resource implementation.
type SearchAttributeResource struct {
	client grpc.ClientConnInterface
}

// SearchAttributeResourceModel defines the data schema for a Temporal search attribute resource.
type SearchAttributeResourceModel struct {
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Namespace types.String `tfsdk:"namespace"`
}

// Metadata sets the metadata for the  search attribute resource, specifically the type name.
func (r *SearchAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_search_attribute"
}

// Schema returns the schema for the Temporal search attribute resource.
func (r *SearchAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Temporal Search Attribute resource",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Search Attribute Name",
				Required:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Search Attribute Indexed Value Type, which defines the type of data stored in the attribute",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("Unspecified", "Text", "Keyword", "Int", "Double", "Bool", "Datetime", "KeywordList"), // Ensure only valid types are used
				},
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "Namespace with which the Search Attribute is associated",
				Optional:            true,
				Default:             stringdefault.StaticString("default"),
				Computed:            true,
			},
		},
	}
}

// Configure sets up the search attribute resource configuration.
func (r *SearchAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	tflog.Info(ctx, "Configuring Temporal Search Attribute Resource")

	// Prevent panic if the provider has not been configured yet
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
	tflog.Info(ctx, "Configured Temporal Search Attribute client", map[string]any{"success": true})
}

// Create is responsible for creating a new search attribute in Temporal.
func (r *SearchAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SearchAttributeResourceModel

	client := operatorservice.NewOperatorServiceClient(r.client)

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Serialize the full create cycle per namespace. Temporal's AddSearchAttributes
	// performs a non-atomic read-modify-write on cluster metadata — concurrent calls
	// return success but silently lose updates. The lock covers check + add + await
	// so the next resource only starts after the previous one is fully confirmed.
	var createErr error
	withNamespaceLock(data.Namespace.ValueString(), func() {
		createErr = r.createSearchAttribute(ctx, client, data)
	})

	if createErr != nil {
		if _, ok := createErr.(*serviceerror.AlreadyExists); ok {
			resp.Diagnostics.AddError("Request Error", "Search attribute with that name is already registered: "+createErr.Error())
		} else {
			resp.Diagnostics.AddError("Request error", "Search attribute creation failed: "+createErr.Error())
		}
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("The search attribute: %s of type %s is successfully created", data.Name, data.Type.String()))
}

// createSearchAttribute performs the check-then-add-then-await sequence.
// Must be called under the namespace lock.
func (r *SearchAttributeResource) createSearchAttribute(
	ctx context.Context,
	client operatorservice.OperatorServiceClient,
	data SearchAttributeResourceModel,
) error {
	// Check if the attribute already exists in the custom attributes
	existingAttrs, err := client.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
		Namespace: data.Namespace.ValueString(),
	})
	if err != nil {
		return err
	}
	if _, exists := existingAttrs.CustomAttributes[data.Name.ValueString()]; exists {
		return &serviceerror.AlreadyExists{Message: "search attribute already exists"}
	}

	// Create attribute
	indexedValueType, _ := enums.IndexedValueTypeFromString(data.Type.ValueString())

	_, err = client.AddSearchAttributes(ctx, &operatorservice.AddSearchAttributesRequest{
		Namespace:        data.Namespace.ValueString(),
		SearchAttributes: map[string]enums.IndexedValueType{data.Name.ValueString(): indexedValueType},
	})
	if err != nil {
		return err
	}

	// Wait for the attribute to become visible.
	return awaitSearchAttribute(ctx, client, data, searchAttributeAwaitTimeout)
}

// Read is responsible for reading the current state of a Temporal search attribute.
// It fetches the current configuration of the search attribute and updates the Terraform state.
func (r *SearchAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SearchAttributeResourceModel

	client := operatorservice.NewOperatorServiceClient(r.client)

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attributes, err := client.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
		Namespace: state.Namespace.ValueString(),
	})

	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read search attribute info, got error: %s", err))
		return
	}

	// Attempt to find search attribute
	attr, ok := attributes.CustomAttributes[state.Name.ValueString()]
	if !ok {
		// Delete resource from state if not found in underlying system
		tflog.Info(ctx, "Resource not found, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	data := &SearchAttributeResourceModel{
		Name:      state.Name,
		Namespace: state.Namespace,
		Type:      types.StringValue(attr.String()),
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, "Read a Temporal search attribute resource")
}

// Update is a no-op method that handles update calls without making any changes.
func (r *SearchAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {

	tflog.Warn(ctx, "Update operation called, but updates are not supported for this resource.")

}

// Delete removes a Temporal search attribute from both Temporal and the Terraform state.
func (r *SearchAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SearchAttributeResourceModel

	client := operatorservice.NewOperatorServiceClient(r.client)

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Serialize under namespace lock to prevent metadata races.
	var deleteErr error
	withNamespaceLock(data.Namespace.ValueString(), func() {
		_, deleteErr = client.RemoveSearchAttributes(ctx, &operatorservice.RemoveSearchAttributesRequest{
			Namespace:        data.Namespace.ValueString(),
			SearchAttributes: []string{data.Name.ValueString()},
		})
	})

	if deleteErr != nil {
		if _, ok := deleteErr.(*serviceerror.NotFound); ok {
			resp.Diagnostics.AddError("Request error", "Search attribute not found: "+deleteErr.Error())
			return
		}
		resp.Diagnostics.AddError("Request error", "Unable to delete search attribute "+deleteErr.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Successfully deleted search attribute: %s", data.Name.ValueString()))
}

// ImportState allows existing Temporal search attributes to be imported into the Terraform state.
// Importing system attributes is not supported.
func (r *SearchAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {

	// Expected request ID format is either 'namespace:search_attribute_name' or 'search_attribute_name'
	// Ex: 'default:CustomBool' or 'customBool'
	// If no namespace is provided, 'default' will be used

	var namespace, attributeName string

	idTokens := strings.Split(req.ID, ":")
	switch len(idTokens) {
	case 1:
		// One part: Attribute name with default namespace ('CustomBool')
		namespace = "default"
		attributeName = idTokens[0]
	case 2:
		// Two parts: the first is namespace, second is attribute name ('default:CustomBool')
		namespace = idTokens[0]
		attributeName = idTokens[1]
	default:
		// If neither, return an error
		resp.Diagnostics.AddError("Invalid ID format", "Expected 'namespace:search_attribute_name' or just 'search_attribute_name'.")
		return
	}

	// Fetch the search attribute details
	client := operatorservice.NewOperatorServiceClient(r.client)
	attrRequest := &operatorservice.ListSearchAttributesRequest{
		Namespace: namespace,
	}

	attributes, err := client.ListSearchAttributes(ctx, attrRequest)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read search attribute info, got error: %s", err))
		return
	}

	// Find the specific attribute and get its type
	var attributeType enums.IndexedValueType
	var found bool

	if attributeType, found = attributes.GetCustomAttributes()[attributeName]; !found {
		resp.Diagnostics.AddError("Not Found", fmt.Sprintf("Custom Search Attribute '%s' not found in namespace '%s'", attributeName, namespace))
		return
	}

	// Set the ID and other attributes needed for managing the resource
	diags := resp.State.Set(ctx, &SearchAttributeResourceModel{
		Name:      types.StringValue(attributeName),
		Namespace: types.StringValue(namespace),
		Type:      types.StringValue(attributeType.String()),
	})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Imported search attribute %s of type %s successfully", attributeName, attributeType.String()))
}
