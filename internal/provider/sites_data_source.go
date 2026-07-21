// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &sitesDataSource{}
var _ datasource.DataSourceWithConfigure = &sitesDataSource{}

func NewSitesDataSource() datasource.DataSource {
	return &sitesDataSource{}
}

type sitesDataSource struct {
	data *providerData
}

type siteModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type sitesDataSourceModel struct {
	Sites []siteModel `tfsdk:"sites"`
}

func (d *sitesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sites"
}

func (d *sitesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all sites on the Omada controller.",
		Attributes: map[string]schema.Attribute{
			"sites": schema.ListNestedAttribute{
				MarkdownDescription: "All sites on the controller.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "The site ID."},
						"name": schema.StringAttribute{Computed: true, MarkdownDescription: "The site name."},
					},
				},
			},
		},
	}
}

func (d *sitesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *sitesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	sites, err := d.data.client.ListSites(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada sites", err.Error())
		return
	}

	var state sitesDataSourceModel
	for _, s := range sites {
		state.Sites = append(state.Sites, siteModel{
			ID:   types.StringValue(s.ID),
			Name: types.StringValue(s.Name),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
