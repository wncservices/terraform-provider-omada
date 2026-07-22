// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &ipGroupResource{}
	_ resource.ResourceWithConfigure   = &ipGroupResource{}
	_ resource.ResourceWithImportState = &ipGroupResource{}
)

func NewIPGroupResource() resource.Resource {
	return &ipGroupResource{}
}

type ipGroupResource struct {
	data *providerData
}

type ipGroupEntryModel struct {
	IP          types.String `tfsdk:"ip"`
	Mask        types.Int64  `tfsdk:"mask"`
	Description types.String `tfsdk:"description"`
}

type ipGroupResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`
	Name   types.String `tfsdk:"name"`
	IPList types.List   `tfsdk:"ip_list"`
}

var ipGroupEntryObjType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"ip": types.StringType, "mask": types.Int64Type, "description": types.StringType,
}}

func (r *ipGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_group"
}

func (r *ipGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a reusable IP group on the Omada controller (Settings → Profiles → Groups), usable as a source/destination in firewall ACLs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Controller-assigned group ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name. Defaults to the primary site. Changing this forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Resolved site ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name.",
				Required:            true,
			},
			"ip_list": schema.ListNestedAttribute{
				MarkdownDescription: "IPv4 CIDR entries in the group.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip":   schema.StringAttribute{Required: true, MarkdownDescription: "Network address, e.g. `10.10.10.0`."},
						"mask": schema.Int64Attribute{Required: true, MarkdownDescription: "Prefix length (1-32)."},
						"description": schema.StringAttribute{
							Optional: true, Computed: true, MarkdownDescription: "Optional per-entry description.",
							Default: stringdefault.StaticString(""),
						},
					},
				},
			},
		},
	}
}

func (r *ipGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *ipGroupResource) siteName(m ipGroupResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *ipGroupResource) inputFrom(ctx context.Context, m ipGroupResourceModel) (*omada.IPGroupInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	var entries []ipGroupEntryModel
	diags.Append(m.IPList.ElementsAs(ctx, &entries, false)...)
	list := make([]omada.IPGroupEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, omada.IPGroupEntry{
			IP: e.IP.ValueString(), Mask: int(e.Mask.ValueInt64()), Description: e.Description.ValueString(),
		})
	}
	return &omada.IPGroupInput{Name: m.Name.ValueString(), IPList: list}, diags
}

func (r *ipGroupResource) apply(ctx context.Context, g *omada.IPGroup, m *ipGroupResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(g.GroupID)
	m.Name = types.StringValue(g.Name)
	entries := make([]ipGroupEntryModel, 0, len(g.IPList))
	for _, e := range g.IPList {
		entries = append(entries, ipGroupEntryModel{
			IP: types.StringValue(e.IP), Mask: types.Int64Value(int64(e.Mask)), Description: types.StringValue(e.Description),
		})
	}
	list, d := types.ListValueFrom(ctx, ipGroupEntryObjType, entries)
	diags.Append(d...)
	m.IPList = list
	return diags
}

func (r *ipGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ipGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreateIPGroup(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create IP group", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ipGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ipGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := state.SiteID.ValueString()
	if siteID == "" {
		var err error
		if siteID, err = r.data.client.ResolveSiteID(ctx, r.siteName(state)); err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
	}
	g, err := r.data.client.GetIPGroup(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, g, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ipGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ipGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateIPGroup(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update IP group", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ipGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ipGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteIPGroup(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete IP group", err.Error())
	}
}

// ImportState accepts "<groupId>" or "<site_name>/<groupId>".
func (r *ipGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
