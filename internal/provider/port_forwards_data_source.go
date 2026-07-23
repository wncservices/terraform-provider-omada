// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &portForwardsDataSource{}
var _ datasource.DataSourceWithConfigure = &portForwardsDataSource{}

func NewPortForwardsDataSource() datasource.DataSource { return &portForwardsDataSource{} }

type portForwardsDataSource struct {
	data *providerData
}

type portForwardModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	Protocol     types.Int64  `tfsdk:"protocol"`
	ExternalPort types.String `tfsdk:"external_port"`
	ForwardIP    types.String `tfsdk:"forward_ip"`
	ForwardPort  types.String `tfsdk:"forward_port"`
}

type portForwardsDataSourceModel struct {
	SiteID       types.String       `tfsdk:"site_id"`
	SiteName     types.String       `tfsdk:"site"`
	PortForwards []portForwardModel `tfsdk:"port_forwards"`
}

func (d *portForwardsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forwards"
}

func (d *portForwardsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists port-forwarding rules for a site — handy for discovering rule IDs to import.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Site ID to query. Provide this or `site`; if neither is set, the provider's default site is used.",
				Optional:            true,
				Computed:            true,
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name to query (resolved to an ID). Ignored if `site_id` is set.",
				Optional:            true,
			},
			"port_forwards": schema.ListNestedAttribute{
				MarkdownDescription: "The port-forwarding rules defined on the site.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "The rule ID (use this to import `omada_port_forward`)."},
						"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "The rule name."},
						"enabled":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rule is enabled."},
						"protocol":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Protocol: 1=TCP, 2=UDP, 3=TCP+UDP."},
						"external_port": schema.StringAttribute{Computed: true, MarkdownDescription: "External port or range."},
						"forward_ip":    schema.StringAttribute{Computed: true, MarkdownDescription: "Internal destination IP."},
						"forward_port":  schema.StringAttribute{Computed: true, MarkdownDescription: "Internal destination port or range."},
					},
				},
			},
		},
	}
}

func (d *portForwardsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
		return
	}
	d.data = data
}

func (d *portForwardsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config portForwardsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID, diags := d.resolveSiteID(ctx, config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := d.data.client.ListPortForwards(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada port forwards", err.Error())
		return
	}

	state := portForwardsDataSourceModel{
		SiteID:   types.StringValue(siteID),
		SiteName: config.SiteName,
	}
	for _, r := range rules {
		state.PortForwards = append(state.PortForwards, portForwardModel{
			ID:           types.StringValue(r.ID),
			Name:         types.StringValue(r.Name),
			Enabled:      types.BoolValue(r.Status),
			Protocol:     types.Int64Value(int64(r.Protocol)),
			ExternalPort: types.StringValue(r.ExternalPort),
			ForwardIP:    types.StringValue(r.ForwardIP),
			ForwardPort:  types.StringValue(r.ForwardPort),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *portForwardsDataSource) resolveSiteID(ctx context.Context, config portForwardsDataSourceModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !config.SiteID.IsNull() && config.SiteID.ValueString() != "" {
		return config.SiteID.ValueString(), diags
	}
	name := d.data.defaultSite
	if !config.SiteName.IsNull() && config.SiteName.ValueString() != "" {
		name = config.SiteName.ValueString()
	}
	id, err := d.data.client.ResolveSiteID(ctx, name)
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", diags
	}
	return id, diags
}
