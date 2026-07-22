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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &wirelessResource{}
	_ resource.ResourceWithConfigure   = &wirelessResource{}
	_ resource.ResourceWithImportState = &wirelessResource{}
)

func NewWirelessNetworkResource() resource.Resource { return &wirelessResource{} }

type wirelessResource struct{ data *providerData }

type wirelessResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	SiteID      types.String `tfsdk:"site_id"`
	WLANGroupID types.String `tfsdk:"wlan_group_id"`
	Name        types.String `tfsdk:"name"`
	Band        types.Int64  `tfsdk:"band"`
	Security    types.Int64  `tfsdk:"security"`
	PSK         types.String `tfsdk:"psk"`
	Broadcast   types.Bool   `tfsdk:"broadcast"`
	VLANEnable  types.Bool   `tfsdk:"vlan_enable"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	GuestNet    types.Bool   `tfsdk:"guest_net"`
}

func (r *wirelessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wireless_network"
}

func (r *wirelessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a wireless SSID within a WLAN group. A managed subset of fields is exposed; other SSID settings are preserved on update.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":          schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"wlan_group_id": schema.StringAttribute{Required: true, MarkdownDescription: "WLAN group this SSID belongs to. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":          schema.StringAttribute{Required: true, MarkdownDescription: "The SSID (network name)."},
			"band":          schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(7), MarkdownDescription: "Radio band bitmask: 1=2.4GHz, 2=5GHz, 4=6GHz (7=all)."},
			"security":      schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(3), MarkdownDescription: "Security mode: 0=open, 3=WPA2/WPA3-PSK."},
			"psk":           schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Pre-shared key (WiFi password). Write-only from state's perspective; not refreshed from the controller."},
			"broadcast":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the SSID is broadcast (visible)."},
			"vlan_enable":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether the SSID is tagged to a VLAN."},
			"vlan_id":       schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), MarkdownDescription: "VLAN ID when vlan_enable is true."},
			"guest_net":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether this is a guest network."},
		},
	}
}

func (r *wirelessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *wirelessResource) siteName(m wirelessResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *wirelessResource) fields(m wirelessResourceModel) map[string]any {
	return map[string]any{
		"name":           m.Name.ValueString(),
		"band":           m.Band.ValueInt64(),
		"security":       m.Security.ValueInt64(),
		"broadcast":      m.Broadcast.ValueBool(),
		"vlanEnable":     m.VLANEnable.ValueBool(),
		"vlanId":         m.VLANID.ValueInt64(),
		"guestNetEnable": m.GuestNet.ValueBool(),
	}
}

// apply refreshes state from the controller. psk is not returned by the API in a
// comparable form, so it is left as-is (write-only).
func (r *wirelessResource) apply(w *omada.WirelessNetwork, m *wirelessResourceModel) {
	m.ID = types.StringValue(w.ID)
	m.Name = types.StringValue(w.Name)
	m.Band = types.Int64Value(int64(w.Band))
	m.Security = types.Int64Value(int64(w.Security))
	m.Broadcast = types.BoolValue(w.Broadcast)
	m.VLANEnable = types.BoolValue(w.VLANEnable)
	m.VLANID = types.Int64Value(int64(w.VLANID))
	m.GuestNet = types.BoolValue(w.GuestNet)
}

func (r *wirelessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wirelessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	created, err := r.data.client.CreateSSID(ctx, siteID, plan.WLANGroupID.ValueString(), r.fields(plan), plan.PSK.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create SSID", err.Error())
		return
	}
	r.apply(created, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wirelessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wirelessResourceModel
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
	w, err := r.data.client.GetSSID(ctx, siteID, state.WLANGroupID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(w, &state)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wirelessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wirelessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	updated, err := r.data.client.UpdateSSID(ctx, siteID, plan.WLANGroupID.ValueString(), plan.ID.ValueString(), r.fields(plan), plan.PSK.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update SSID", err.Error())
		return
	}
	r.apply(updated, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wirelessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wirelessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete SSID", err.Error())
	}
}

// ImportState accepts "<wlan_group_id>/<ssid_id>" or "<site>/<wlan_group_id>/<ssid_id>".
func (r *wirelessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	switch len(parts) {
	case 3:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
	case 2:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	default:
		resp.Diagnostics.AddError("Invalid import ID", "expected '<wlan_group_id>/<ssid_id>' or '<site>/<wlan_group_id>/<ssid_id>'")
	}
}
