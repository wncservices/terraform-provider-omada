// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &portForwardResource{}
	_ resource.ResourceWithConfigure   = &portForwardResource{}
	_ resource.ResourceWithImportState = &portForwardResource{}
)

func NewPortForwardResource() resource.Resource {
	return &portForwardResource{}
}

type portForwardResource struct {
	data *providerData
}

type portForwardResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Site         types.String `tfsdk:"site"`
	SiteID       types.String `tfsdk:"site_id"`
	Name         types.String `tfsdk:"name"`
	Enable       types.Bool   `tfsdk:"enable"`
	ExternalPort types.String `tfsdk:"external_port"`
	ForwardIP    types.String `tfsdk:"forward_ip"`
	ForwardPort  types.String `tfsdk:"forward_port"`
	Protocol     types.String `tfsdk:"protocol"`
	WANPortIDs   types.List   `tfsdk:"wan_port_ids"`
	DMZ          types.Bool   `tfsdk:"dmz"`
}

var protoToInt = map[string]int{"tcp": 1, "udp": 2, "tcp_udp": 3}
var protoToStr = map[int]string{1: "tcp", 2: "udp", 3: "tcp_udp"}

func (r *portForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_forward"
}

func (r *portForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a NAT port-forwarding rule on the Omada gateway (Settings → Transmission → NAT → Port Forwarding).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Controller-assigned rule ID.",
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
				MarkdownDescription: "Rule name.",
				Required:            true,
			},
			"enable": schema.BoolAttribute{
				MarkdownDescription: "Whether the rule is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"external_port": schema.StringAttribute{
				MarkdownDescription: "External (WAN) port or range, e.g. `443` or `8000-8010`.",
				Required:            true,
			},
			"forward_ip": schema.StringAttribute{
				MarkdownDescription: "Internal LAN IP to forward to.",
				Required:            true,
			},
			"forward_port": schema.StringAttribute{
				MarkdownDescription: "Internal port or range.",
				Required:            true,
			},
			"protocol": schema.StringAttribute{
				MarkdownDescription: "One of `tcp`, `udp`, or `tcp_udp`.",
				Required:            true,
			},
			"wan_port_ids": schema.ListAttribute{
				MarkdownDescription: "WAN port(s) the rule applies to. Defaults to the WAN port used by existing rules; populated on import.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"dmz": schema.BoolAttribute{
				MarkdownDescription: "Whether this is a DMZ rule.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *portForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *portForwardResource) siteName(m portForwardResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *portForwardResource) inputFrom(ctx context.Context, siteID string, m portForwardResourceModel) (*omada.PortForwardInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	proto, ok := protoToInt[strings.ToLower(m.Protocol.ValueString())]
	if !ok {
		diags.AddAttributeError(path.Root("protocol"), "Invalid protocol", "protocol must be one of tcp, udp, tcp_udp")
		return nil, diags
	}

	wanPorts, d := stringSlice(ctx, m.WANPortIDs)
	diags.Append(d...)
	if len(wanPorts) == 0 {
		resolved, err := r.data.client.DefaultWANPortIDs(ctx, siteID)
		if err != nil {
			diags.AddError("Unable to resolve WAN port", err.Error())
			return nil, diags
		}
		wanPorts = resolved
	}

	return &omada.PortForwardInput{
		Name:          m.Name.ValueString(),
		Status:        m.Enable.ValueBool(),
		WANPortIDs:    wanPorts,
		VirtualWANIDs: []string{},
		ExternalPort:  m.ExternalPort.ValueString(),
		ForwardIP:     m.ForwardIP.ValueString(),
		ForwardPort:   m.ForwardPort.ValueString(),
		Protocol:      proto,
		DMZ:           m.DMZ.ValueBool(),
	}, diags
}

func (r *portForwardResource) apply(ctx context.Context, n *omada.PortForward, m *portForwardResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(n.ID)
	m.Name = types.StringValue(n.Name)
	m.Enable = types.BoolValue(n.Status)
	m.ExternalPort = types.StringValue(n.ExternalPort)
	m.ForwardIP = types.StringValue(n.ForwardIP)
	m.ForwardPort = types.StringValue(n.ForwardPort)
	if s, ok := protoToStr[n.Protocol]; ok {
		m.Protocol = types.StringValue(s)
	}
	m.DMZ = types.BoolValue(n.DMZ)
	list, d := stringListValue(ctx, n.WANPortIDs)
	diags.Append(d...)
	m.WANPortIDs = list
	return diags
}

func (r *portForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portForwardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	in, diags := r.inputFrom(ctx, siteID, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreatePortForward(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create port forward", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portForwardResourceModel
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
	rec, err := r.data.client.GetPortForward(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, rec, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portForwardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portForwardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	in, diags := r.inputFrom(ctx, siteID, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdatePortForward(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update port forward", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portForwardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeletePortForward(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete port forward", err.Error())
	}
}

// ImportState accepts "<id>" or "<site_name>/<id>".
func (r *portForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
