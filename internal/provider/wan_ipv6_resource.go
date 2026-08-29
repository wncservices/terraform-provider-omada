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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &wanIPv6Resource{}
	_ resource.ResourceWithConfigure   = &wanIPv6Resource{}
	_ resource.ResourceWithImportState = &wanIPv6Resource{}
)

func NewWANIPv6Resource() resource.Resource { return &wanIPv6Resource{} }

type wanIPv6Resource struct{ data *providerData }

type wanIPv6DynamicModel struct {
	GetIPv6     types.String `tfsdk:"get_ipv6"`
	GetIPv6Type types.Int64  `tfsdk:"get_ipv6_type"`
	Prefix      types.Int64  `tfsdk:"prefix"`
	PDSize      types.Int64  `tfsdk:"pd_size"`
	DNS         types.String `tfsdk:"dns"`
	DNSType     types.Int64  `tfsdk:"dns_type"`
}

var wanIPv6DynamicAttrTypes = map[string]attr.Type{
	"get_ipv6":      types.StringType,
	"get_ipv6_type": types.Int64Type,
	"prefix":        types.Int64Type,
	"pd_size":       types.Int64Type,
	"dns":           types.StringType,
	"dns_type":      types.Int64Type,
}

type wanIPv6ResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Site      types.String `tfsdk:"site"`
	SiteID    types.String `tfsdk:"site_id"`
	PortUUID  types.String `tfsdk:"port_uuid"`
	Enable    types.Bool   `tfsdk:"enable"`
	Proto     types.String `tfsdk:"proto"`
	ProtoType types.Int64  `tfsdk:"proto_type"`
	Dynamic   types.Object `tfsdk:"dynamic"`
}

func (r *wanIPv6Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wan_ipv6"
}

func (r *wanIPv6Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a WAN port's IPv6 connection settings (prefix delegation, address " +
			"acquisition mode).\n\n" +
			"~> **Writes are inferred, not live-validated.** Every other write path in this provider was " +
			"confirmed against a live controller before shipping; this one could not be, because unlike a " +
			"LAN VLAN or a firewall rule, there is only one WAN, and a bad write there drops the site's " +
			"internet connection rather than one segment of it. The read shape is confirmed live. Import an " +
			"existing WAN port and verify a no-op apply before changing anything.\n\n" +
			"WAN ports are physical and cannot be created or deleted — `terraform destroy` only stops " +
			"managing the port, it does not reset its configuration.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name. Defaults to the primary site. Changing this forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"port_uuid": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The WAN port's controller UUID — see the `omada_wan` data source. Changing this forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable":     schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Whether IPv6 is enabled on this WAN port."},
			"proto":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 connection type. Only `\"dynamic\"` (DHCPv6/SLAAC autoconfig, optionally with prefix delegation) is verified."},
			"proto_type": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Controller enum paired with `proto`; `1` observed live for `\"dynamic\"` and undocumented by TP-Link."},
			"dynamic": schema.SingleNestedAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Config used when `proto = \"dynamic\"`.",
				Attributes: map[string]schema.Attribute{
					"get_ipv6":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "IPv6 address acquisition mode, e.g. `\"auto\"`."},
					"get_ipv6_type": schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Controller enum paired with `get_ipv6`; `3` observed live."},
					"prefix":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Whether to request prefix delegation from the ISP (`1` = yes, observed live)."},
					"pd_size":       schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Requested delegated prefix size in bits, e.g. `48`."},
					"dns":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DNS acquisition mode, e.g. `\"dynamic\"`."},
					"dns_type":      schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Controller enum paired with `dns`; `0` observed live."},
				},
			},
		},
	}
}

func (r *wanIPv6Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *wanIPv6Resource) siteName(m wanIPv6ResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *wanIPv6Resource) resolveSite(ctx context.Context, m wanIPv6ResourceModel, diags *diagSink) string {
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

func (m wanIPv6ResourceModel) toAPI(ctx context.Context) (omada.WANIPv6Setting, diag.Diagnostics) {
	var diags diag.Diagnostics
	enable := 0
	if m.Enable.ValueBool() {
		enable = 1
	}
	out := omada.WANIPv6Setting{
		PortUUID:  m.PortUUID.ValueString(),
		Enable:    enable,
		Proto:     m.Proto.ValueString(),
		ProtoType: int(m.ProtoType.ValueInt64()),
	}
	if !m.Dynamic.IsNull() && !m.Dynamic.IsUnknown() {
		var dm wanIPv6DynamicModel
		diags.Append(m.Dynamic.As(ctx, &dm, basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true})...)
		out.Dynamic = &omada.WANIPv6Dynamic{
			GetIPv6: dm.GetIPv6.ValueString(), GetIPv6Type: int(dm.GetIPv6Type.ValueInt64()),
			Prefix: int(dm.Prefix.ValueInt64()), PDSize: int(dm.PDSize.ValueInt64()),
			DNS: dm.DNS.ValueString(), DNSType: int(dm.DNSType.ValueInt64()),
		}
	}
	return out, diags
}

func (m *wanIPv6ResourceModel) refresh(s *omada.WANIPv6Setting) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(s.PortUUID)
	m.PortUUID = types.StringValue(s.PortUUID)
	m.Enable = types.BoolValue(s.Enable != 0)
	m.Proto = types.StringValue(s.Proto)
	m.ProtoType = types.Int64Value(int64(s.ProtoType))

	dyn := types.ObjectNull(wanIPv6DynamicAttrTypes)
	if s.Dynamic != nil {
		v, d := types.ObjectValue(wanIPv6DynamicAttrTypes, map[string]attr.Value{
			"get_ipv6":      types.StringValue(s.Dynamic.GetIPv6),
			"get_ipv6_type": types.Int64Value(int64(s.Dynamic.GetIPv6Type)),
			"prefix":        types.Int64Value(int64(s.Dynamic.Prefix)),
			"pd_size":       types.Int64Value(int64(s.Dynamic.PDSize)),
			"dns":           types.StringValue(s.Dynamic.DNS),
			"dns_type":      types.Int64Value(int64(s.Dynamic.DNSType)),
		})
		diags.Append(d...)
		dyn = v
	}
	m.Dynamic = dyn
	return diags
}

// Create adopts the WAN port named by port_uuid and applies the configured
// IPv6 settings. There is no "create a WAN port" — it is physical.
func (r *wanIPv6Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wanIPv6ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateWANIPv6Setting(ctx, siteID, plan.PortUUID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update WAN IPv6 setting", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.refresh(updated)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wanIPv6Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wanIPv6ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetWANIPv6Setting(ctx, siteID, state.PortUUID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(state.refresh(cur)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wanIPv6Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wanIPv6ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateWANIPv6Setting(ctx, siteID, plan.PortUUID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update WAN IPv6 setting", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.refresh(updated)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops the port from state without touching the WAN — see the
// switch-port resource for the identical reasoning: there is no "delete a
// physical port," and guessing a reset value would be worse than leaving it.
func (r *wanIPv6Resource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"WAN port left as configured",
		"WAN ports are physical and cannot be deleted, so Terraform has only stopped managing this "+
			"one. Its IPv6 settings are unchanged — reset them by hand, or re-import the port, if that "+
			"is not what you wanted.",
	)
}

// ImportState takes the WAN port's UUID, or "<site>/<port_uuid>".
func (r *wanIPv6Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("port_uuid"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
