// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &serviceTypeResource{}
	_ resource.ResourceWithConfigure   = &serviceTypeResource{}
	_ resource.ResourceWithImportState = &serviceTypeResource{}
)

func NewServiceTypeResource() resource.Resource { return &serviceTypeResource{} }

type serviceTypeResource struct{ data *providerData }

type serviceTypeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	SiteID      types.String `tfsdk:"site_id"`
	Name        types.String `tfsdk:"name"`
	Protocol    types.Int64  `tfsdk:"protocol"`
	SourcePorts types.String `tfsdk:"source_ports"`
	DestPorts   types.String `tfsdk:"destination_ports"`
	Description types.String `tfsdk:"description"`
	ICMPType    types.Int64  `tfsdk:"icmp_type"`
	ICMPCode    types.Int64  `tfsdk:"icmp_code"`
	IsDefault   types.Bool   `tfsdk:"is_builtin"`
}

func (r *serviceTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_type"
}

func (r *serviceTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom service type: a reusable protocol/port definition that firewall " +
			"and QoS rules can reference.\n\n" +
			"The controller ships twelve built-ins (`ALL`, `FTP`, `SSH`, `TELNET`, …) marked " +
			"`is_builtin`. Those are reference data — reference them by id, but do not try to manage " +
			"them here; this resource is for custom definitions.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Service type name."},
			"protocol": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Protocol, as a controller enum. On the firmware this was developed against " +
					"`0` is what the built-in TCP services (FTP, SSH, TELNET) use and `2` is what `ALL` uses. " +
					"TP-Link does not document the mapping — check an existing service type with the " +
					"`omada_service_types` listing before guessing.",
			},
			"source_ports":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("0-65535"), MarkdownDescription: "Source port range, e.g. `0-65535`."},
			"destination_ports": schema.StringAttribute{Required: true, MarkdownDescription: "Destination port range, e.g. `8080-8080`."},
			"description":       schema.StringAttribute{Optional: true, MarkdownDescription: "Free-text description."},
			"icmp_type":         schema.Int64Attribute{Optional: true, MarkdownDescription: "ICMP type, for protocols that use it."},
			"icmp_code":         schema.Int64Attribute{Optional: true, MarkdownDescription: "ICMP code, for protocols that use it."},
			"is_builtin":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is one of the controller's built-in service types. Read-only."},
		},
	}
}

func (r *serviceTypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *serviceTypeResource) siteName(m serviceTypeResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *serviceTypeResource) resolveSite(ctx context.Context, m serviceTypeResourceModel, diags *diagSink) string {
	if id := m.SiteID.ValueString(); id != "" {
		return id
	}
	id, err := r.data.client.ResolveSiteID(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return ""
	}
	return id
}

func intPtrFrom(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := int(v.ValueInt64())
	return &i
}

func (m serviceTypeResourceModel) toAPI() omada.ServiceType {
	return omada.ServiceType{
		Name:        m.Name.ValueString(),
		Protocol:    int(m.Protocol.ValueInt64()),
		SourcePorts: m.SourcePorts.ValueString(),
		DestPorts:   m.DestPorts.ValueString(),
		Description: m.Description.ValueString(),
		Type:        intPtrFrom(m.ICMPType),
		Code:        intPtrFrom(m.ICMPCode),
	}
}

func (m *serviceTypeResourceModel) refresh(st *omada.ServiceType) {
	m.ID = types.StringValue(st.ID)
	m.Name = types.StringValue(st.Name)
	m.Protocol = types.Int64Value(int64(st.Protocol))
	m.SourcePorts = types.StringValue(st.SourcePorts)
	m.DestPorts = types.StringValue(st.DestPorts)
	m.IsDefault = types.BoolValue(st.Default)
	if st.Description != "" || !m.Description.IsNull() {
		m.Description = types.StringValue(st.Description)
	}
	// Keep ICMP fields null unless the controller reports them, so a service
	// type that does not use them does not acquire phantom zeroes.
	if st.Type != nil {
		m.ICMPType = types.Int64Value(int64(*st.Type))
	}
	if st.Code != nil {
		m.ICMPCode = types.Int64Value(int64(*st.Code))
	}
}

func (r *serviceTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateServiceType(ctx, siteID, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create service type", err.Error())
		return
	}
	cur, err := r.data.client.GetServiceType(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read service type after create", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetServiceType(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *serviceTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state serviceTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.data.client.UpdateServiceType(ctx, siteID, id, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Unable to update service type", err.Error())
		return
	}
	cur, err := r.data.client.GetServiceType(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read service type after update", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteServiceType(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete service type", err.Error())
	}
}

// ImportState takes the service-type id, or "<site>/<id>".
func (r *serviceTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
