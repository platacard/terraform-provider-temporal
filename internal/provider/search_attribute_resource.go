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

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc"
)

var (
	_ resource.Resource                = &SearchAttributeResource{}
	_ resource.ResourceWithConfigure   = &SearchAttributeResource{}
	_ resource.ResourceWithImportState = &SearchAttributeResource{}
)

// AwaitAddSearchAttributes waits for the completion of AddSearchAttributesRequest using ListSearchAttributes.
func AwaitAddSearchAttributes(ctx context.Context, operatorClient operatorservice.OperatorServiceClient, data SearchAttributeResourceModel) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Verify attribute creation
			attributes, err := operatorClient.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
				Namespace: data.Namespace.ValueString(),
			})
			if err != nil {
				return err
			}

			if _, ok := attributes.CustomAttributes[data.Name.ValueString()]; ok {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

	// Check if the attribute already exists in the custom attributes
	// The API indicates that this should be handled by the AddSearchAttributes() method, but docs seem to be out of date
	// Support thread: https://temporalio.slack.com/archives/CTDTU3J4T/p1721255485197359

	existingAttrs, err := client.ListSearchAttributes(ctx, &operatorservice.ListSearchAttributesRequest{
		Namespace: data.Namespace.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Client Error", "Failed to list existing search attributes: "+err.Error())
		return
	}
	if _, exists := existingAttrs.CustomAttributes[data.Name.ValueString()]; exists {
		resp.Diagnostics.AddError("Already Exists", "Search attribute with the provided name already exists and cannot be created again.")
		return
	}

	// Create attribute
	indexedValueType, _ := enums.IndexedValueTypeFromString(data.Type.ValueString())

	request := &operatorservice.AddSearchAttributesRequest{
		Namespace:        data.Namespace.ValueString(),
		SearchAttributes: map[string]enums.IndexedValueType{data.Name.ValueString(): indexedValueType},
	}
	_, err = client.AddSearchAttributes(ctx, request)
	if err != nil {
		if _, ok := err.(*serviceerror.AlreadyExists); ok {
			resp.Diagnostics.AddError("Request Error", "Search attribute with that name is already registered: "+err.Error())
			return
		}
		resp.Diagnostics.AddError("Request error", "Search attribute creation failed: "+err.Error())
		return
	}

	err = AwaitAddSearchAttributes(ctx, client, data)
	if err != nil {
		resp.Diagnostics.AddError("Request Error", "Error awaiting results: "+err.Error())
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("The search attribute: %s of type %s is successfully created", data.Name, data.Type.String()))
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

	// Delete request
	request := &operatorservice.RemoveSearchAttributesRequest{
		Namespace:        data.Namespace.ValueString(),
		SearchAttributes: []string{data.Name.ValueString()},
	}

	_, err := client.RemoveSearchAttributes(ctx, request)

	if err != nil {
		if _, ok := err.(*serviceerror.NotFound); ok {
			resp.Diagnostics.AddError("Request error", "Search attribute not found: "+err.Error())
			return
		}
		resp.Diagnostics.AddError("Request error", "Unable to delete search attribute "+err.Error())
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
