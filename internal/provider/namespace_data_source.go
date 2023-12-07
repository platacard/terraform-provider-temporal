package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/numbervalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ datasource.DataSource              = &NamespaceDataSource{}
	_ datasource.DataSourceWithConfigure = &NamespaceDataSource{}
)

func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

// NamespaceDataSource defines the data source implementation.
type NamespaceDataSource struct {
	client workflowservice.WorkflowServiceClient
}

// NamespaceDataSourceModel describes the data source data model.
type NamespaceDataSourceModel struct {
	Name                    string   `tfsdk:"name"`
	Id                      string   `tfsdk:"id"`
	Description             string   `tfsdk:"description"`
	OwnerEmail              string   `tfsdk:"owner_email"`
	State                   string   `tfsdk:"state"`
	ActiveClusterName       string   `tfsdk:"active_cluster_name"`
	Clusters                []string `tfsdk:"clusters"`
	HistoryArchivalState    string   `tfsdk:"history_archival_state"`
	VisibilityArchivalState string   `tfsdk:"visibility_archival_state"`
	IsGlobalNamespace       bool     `tfsdk:"is_global_namespace"`
	FailoverVersion         int      `tfsdk:"failover_version"`
	FailoverHistory         []string `tfsdk:"failover_history"`
}

func (d *NamespaceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (d *NamespaceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Namespace data source",

		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Namespace name",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Namespace identifier",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.Any(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Namespace Description",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"owner_email": schema.StringAttribute{
				MarkdownDescription: "Namespace Owner Email",
				Computed:            true,
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_@]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "State of Namespace",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.Any(),
				},
			},
			"active_cluster_name": schema.StringAttribute{
				MarkdownDescription: "Active Cluster Name",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"clusters": schema.ListAttribute{
				MarkdownDescription: "Temporal Clusters",
				Computed:            true,
				ElementType:         types.StringType,
				Validators: []validator.List{
					listvalidator.Any(),
				},
			},
			"history_archival_state": schema.StringAttribute{
				MarkdownDescription: "History Archival State",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"visibility_archival_state": schema.StringAttribute{
				MarkdownDescription: "Visibility Archival State",
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`),
						"must contain only lowercase/uppercase alphanumeric characters, numbers, - and _",
					),
				},
			},
			"is_global_namespace": schema.BoolAttribute{
				MarkdownDescription: "Namespace is Global",
				Computed:            true,
				Validators: []validator.Bool{
					boolvalidator.Any(),
				},
			},
			"failover_version": schema.NumberAttribute{
				MarkdownDescription: "Failover Version",
				Computed:            true,
				Validators: []validator.Number{
					numbervalidator.Any(),
				},
			},
			"failover_history": schema.ListAttribute{
				MarkdownDescription: "Failover History",
				ElementType:         types.StringType,
				Computed:            true,
				Validators: []validator.List{
					listvalidator.Any(),
				},
			},
		},
	}
}

func (d *NamespaceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	connection, ok := req.ProviderData.(grpc.ClientConnInterface)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected grpc.ClientConnInterface), got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	client := workflowservice.NewWorkflowServiceClient(connection)

	d.client = client
}

func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data *NamespaceDataSourceModel
	var name string
	diags := req.Config.GetAttribute(ctx, path.Root("name"), &name)
	resp.Diagnostics.Append(diags...)
	ns, err := d.client.DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: name,
	})
	fmt.Println("Hurma", ns)
	data = &NamespaceDataSourceModel{
		Name:                    ns.NamespaceInfo.GetName(),
		Id:                      ns.NamespaceInfo.GetId(),
		Description:             ns.NamespaceInfo.GetDescription(),
		OwnerEmail:              ns.NamespaceInfo.GetOwnerEmail(),
		State:                   enums.NamespaceState_name[int32(ns.NamespaceInfo.GetState())],
		ActiveClusterName:       ns.GetReplicationConfig().GetActiveClusterName(),
		HistoryArchivalState:    enums.ArchivalState_name[int32(ns.Config.GetHistoryArchivalState())],
		VisibilityArchivalState: enums.ArchivalState_name[int32(ns.Config.GetVisibilityArchivalState())],
		IsGlobalNamespace:       ns.GetIsGlobalNamespace(),
		FailoverVersion:         int(ns.GetFailoverVersion()),
	}

	for _, clusters := range ns.GetReplicationConfig().GetClusters() {
		data.Clusters = append(data.Clusters, clusters.ClusterName)
	}
	for _, failover := range ns.GetFailoverHistory() {
		data.Clusters = append(data.Clusters, failover.String())
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read Namespace, got error: %s", err))
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
