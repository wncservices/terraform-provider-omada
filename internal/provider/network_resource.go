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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &networkResource{}
	_ resource.ResourceWithConfigure   = &networkResource{}
	_ resource.ResourceWithImportState = &networkResource{}
)

func NewNetworkResource() resource.Resource { return &networkResource{} }

type networkResource struct{ data *providerData }

type networkResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Site          types.String `tfsdk:"site"`
	SiteID        types.String `tfsdk:"site_id"`
	Name          types.String `tfsdk:"name"`
	Purpose       types.String `tfsdk:"purpose"`
	VLANID        types.Int64  `tfsdk:"vlan_id"`
	VLANType      types.Int64  `tfsdk:"vlan_type"`
	Application   types.Int64  `tfsdk:"application"`
	GatewaySubnet types.String `tfsdk:"gateway_subnet"`
	InterfaceIDs  types.List   `tfsdk:"interface_ids"`

	Isolation         types.Bool  `tfsdk:"isolation"`
	AllLan            types.Bool  `tfsdk:"all_lan"`
	Portal            types.Bool  `tfsdk:"portal_enable"`
	RateLimit         types.Bool  `tfsdk:"rate_limit_enable"`
	QosQueue          types.Bool  `tfsdk:"qos_queue_enable"`
	AccessControlRule types.Bool  `tfsdk:"access_control_rule"`
	ArpDetection      types.Bool  `tfsdk:"arp_detection_enable"`
	IGMPSnoop         types.Bool  `tfsdk:"igmp_snoop_enable"`
	FastLeave         types.Bool  `tfsdk:"fast_leave_enable"`
	MLDSnoop          types.Bool  `tfsdk:"mld_snoop_enable"`
	DHCPL2Relay       types.Bool  `tfsdk:"dhcp_l2_relay_enable"`
	DHCPGuard         types.Bool  `tfsdk:"dhcp_guard_enable"`
	DHCPv6Guard       types.Bool  `tfsdk:"dhcpv6_guard_enable"`
	IPv6ConfigEnable  types.Int64 `tfsdk:"ipv6_config_enable"`

	DHCPEnabled   types.Bool   `tfsdk:"dhcp_enabled"`
	DHCPStart     types.String `tfsdk:"dhcp_start"`
	DHCPEnd       types.String `tfsdk:"dhcp_end"`
	DHCPLeaseTime types.Int64  `tfsdk:"dhcp_lease_time"`
	DHCPDNSMode   types.String `tfsdk:"dhcp_dns_mode"`
	DHCPOptions   types.List   `tfsdk:"dhcp_options"`
}

var dhcpOptionAttrTypes = map[string]attr.Type{
	"code":  types.Int64Type,
	"type":  types.Int64Type,
	"value": types.StringType,
}

type dhcpOptionModel struct {
	Code  types.Int64  `tfsdk:"code"`
	Type  types.Int64  `tfsdk:"type"`
	Value types.String `tfsdk:"value"`
}

func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	b := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	i := func(desc string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a LAN network (VLAN) on the Omada controller, including its DHCP server, DHCP options and per-VLAN switching/security toggles.\n\n" +
			"Unset attributes keep their current controller value, and derived fields (address-pool ranges, counters) are preserved on update.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Controller-assigned network ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name. Defaults to the controller's primary site. Changing this forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Network name."},
			"vlan_id": schema.Int64Attribute{Required: true, MarkdownDescription: "VLAN ID (1-4094)."},
			"purpose": schema.StringAttribute{
				MarkdownDescription: "`interface` for a routed VLAN with a gateway + DHCP, or `vlan` for an L2-only VLAN.",
				Optional:            true, Computed: true,
				Default: stringdefault.StaticString("interface"),
			},
			"vlan_type":   i("VLAN type code."),
			"application": i("Application code."),
			"gateway_subnet": schema.StringAttribute{
				MarkdownDescription: "Gateway IP + subnet in CIDR, e.g. `10.10.30.1/24`. Only for `interface` networks.",
				Optional:            true, Computed: true,
			},
			"interface_ids": schema.ListAttribute{
				MarkdownDescription: "Gateway LAN interface IDs this network attaches to.",
				ElementType:         types.StringType,
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},

			"isolation":            b("Isolate this network from other LANs (guest isolation)."),
			"all_lan":              b("Applies to all LANs."),
			"portal_enable":        b("Captive portal on this network."),
			"rate_limit_enable":    b("Rate limiting on this network."),
			"qos_queue_enable":     b("QoS queueing."),
			"access_control_rule":  b("Whether ACL rules apply to this network."),
			"arp_detection_enable": b("ARP inspection/detection."),
			"igmp_snoop_enable":    b("IGMP snooping."),
			"fast_leave_enable":    b("IGMP fast-leave."),
			"mld_snoop_enable":     b("MLD snooping (IPv6)."),
			"dhcp_l2_relay_enable": b("DHCP L2 relay."),
			"dhcp_guard_enable":    b("Rogue-DHCP protection (distinct from the DHCP server)."),
			"dhcpv6_guard_enable":  b("Rogue-DHCPv6 protection."),
			"ipv6_config_enable":   i("IPv6 configuration mode for the network (0 = disabled)."),

			"dhcp_enabled":    b("Enable the DHCP server on this network."),
			"dhcp_start":      schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "First address of the DHCP pool."},
			"dhcp_end":        schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Last address of the DHCP pool."},
			"dhcp_lease_time": i("DHCP lease time (minutes)."),
			"dhcp_dns_mode":   schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "DHCP DNS mode, e.g. `auto`."},
			"dhcp_options": schema.ListNestedAttribute{
				MarkdownDescription: "DHCP options handed out on this network (e.g. code 138 for a controller address).",
				Optional:            true, Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"code":  schema.Int64Attribute{Required: true, MarkdownDescription: "DHCP option code."},
						"type":  schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Option value type."},
						"value": schema.StringAttribute{Required: true, MarkdownDescription: "Option value."},
					},
				},
			},
		},
	}
}

func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *networkResource) siteName(m networkResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// fieldsFrom builds the API payload from known model values.
func (r *networkResource) fieldsFrom(ctx context.Context, m networkResourceModel) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	f := map[string]any{
		"name":    m.Name.ValueString(),
		"purpose": m.Purpose.ValueString(),
		"vlan":    m.VLANID.ValueInt64(),
	}
	putBool := func(key string, v types.Bool) {
		if !v.IsNull() && !v.IsUnknown() {
			f[key] = v.ValueBool()
		}
	}
	putInt := func(key string, v types.Int64) {
		if !v.IsNull() && !v.IsUnknown() {
			f[key] = v.ValueInt64()
		}
	}
	putInt("vlanType", m.VLANType)
	putInt("application", m.Application)
	if !m.GatewaySubnet.IsNull() && !m.GatewaySubnet.IsUnknown() && m.GatewaySubnet.ValueString() != "" {
		f["gatewaySubnet"] = m.GatewaySubnet.ValueString()
	}
	if ids, d := stringSlice(ctx, m.InterfaceIDs); d == nil || !d.HasError() {
		diags.Append(d...)
		if ids != nil {
			f["interfaceIds"] = ids
		}
	}
	putBool("isolation", m.Isolation)
	putBool("allLan", m.AllLan)
	putBool("portal", m.Portal)
	putBool("rateLimit", m.RateLimit)
	putBool("qosQueueEnable", m.QosQueue)
	putBool("accessControlRule", m.AccessControlRule)
	putBool("arpDetectionEnable", m.ArpDetection)
	putBool("igmpSnoopEnable", m.IGMPSnoop)
	putBool("fastLeaveEnable", m.FastLeave)
	putBool("mldSnoopEnable", m.MLDSnoop)
	putBool("dhcpL2RelayEnable", m.DHCPL2Relay)
	if !m.DHCPGuard.IsNull() && !m.DHCPGuard.IsUnknown() {
		f["dhcpGuard"] = map[string]any{"enable": m.DHCPGuard.ValueBool()}
	}
	if !m.DHCPv6Guard.IsNull() && !m.DHCPv6Guard.IsUnknown() {
		f["dhcpv6Guard"] = map[string]any{"enable": m.DHCPv6Guard.ValueBool()}
	}
	if !m.IPv6ConfigEnable.IsNull() && !m.IPv6ConfigEnable.IsUnknown() {
		f["lanNetworkIpv6Config"] = map[string]any{"enable": m.IPv6ConfigEnable.ValueInt64()}
	}

	dhcp := map[string]any{}
	if !m.DHCPEnabled.IsNull() && !m.DHCPEnabled.IsUnknown() {
		dhcp["enable"] = m.DHCPEnabled.ValueBool()
	}
	if !m.DHCPStart.IsNull() && !m.DHCPStart.IsUnknown() && m.DHCPStart.ValueString() != "" {
		dhcp["ipaddrStart"] = m.DHCPStart.ValueString()
	}
	if !m.DHCPEnd.IsNull() && !m.DHCPEnd.IsUnknown() && m.DHCPEnd.ValueString() != "" {
		dhcp["ipaddrEnd"] = m.DHCPEnd.ValueString()
	}
	if !m.DHCPLeaseTime.IsNull() && !m.DHCPLeaseTime.IsUnknown() {
		dhcp["leasetime"] = m.DHCPLeaseTime.ValueInt64()
	}
	if !m.DHCPDNSMode.IsNull() && !m.DHCPDNSMode.IsUnknown() && m.DHCPDNSMode.ValueString() != "" {
		dhcp["dhcpns"] = m.DHCPDNSMode.ValueString()
	}
	if !m.DHCPOptions.IsNull() && !m.DHCPOptions.IsUnknown() {
		var opts []dhcpOptionModel
		diags.Append(m.DHCPOptions.ElementsAs(ctx, &opts, false)...)
		list := make([]map[string]any, 0, len(opts))
		for _, o := range opts {
			list = append(list, map[string]any{
				"code": o.Code.ValueInt64(), "type": o.Type.ValueInt64(), "value": o.Value.ValueString(),
			})
		}
		dhcp["options"] = list
	}
	if len(dhcp) > 0 {
		f["dhcpSettings"] = dhcp
	}
	return f, diags
}

func (r *networkResource) apply(ctx context.Context, n *omada.Network, m *networkResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(n.ID)
	m.Name = types.StringValue(n.Name)
	m.Purpose = types.StringValue(n.Purpose)
	m.VLANID = types.Int64Value(int64(n.VLANID))
	m.VLANType = types.Int64Value(int64(n.VLANType))
	m.Application = types.Int64Value(int64(n.Application))
	m.GatewaySubnet = types.StringValue(n.GatewaySubnet)

	m.Isolation = types.BoolValue(n.Isolation)
	m.AllLan = types.BoolValue(n.AllLan)
	m.Portal = types.BoolValue(n.Portal)
	m.RateLimit = types.BoolValue(n.RateLimit)
	m.QosQueue = types.BoolValue(n.QosQueueEnable)
	m.AccessControlRule = types.BoolValue(n.AccessControlRule)
	m.ArpDetection = types.BoolValue(n.ArpDetection)
	m.IGMPSnoop = types.BoolValue(n.IGMPSnoop)
	m.FastLeave = types.BoolValue(n.FastLeave)
	m.MLDSnoop = types.BoolValue(n.MLDSnoop)
	m.DHCPL2Relay = types.BoolValue(n.DHCPL2Relay)
	m.DHCPGuard = types.BoolValue(n.DHCPGuard.Enable)
	m.DHCPv6Guard = types.BoolValue(n.DHCPv6Guard.Enable)
	m.IPv6ConfigEnable = types.Int64Value(int64(n.IPv6Config.Enable))

	m.DHCPEnabled = types.BoolValue(n.DHCPSettings.Enable)
	m.DHCPStart = types.StringValue(n.DHCPSettings.IPAddrStart)
	m.DHCPEnd = types.StringValue(n.DHCPSettings.IPAddrEnd)
	m.DHCPLeaseTime = types.Int64Value(int64(n.DHCPSettings.LeaseTime))
	m.DHCPDNSMode = types.StringValue(n.DHCPSettings.DNSMode)

	ids := n.InterfaceIDs
	if ids == nil {
		ids = []string{}
	}
	list, d := stringListValue(ctx, ids)
	diags.Append(d...)
	m.InterfaceIDs = list

	elems := make([]attr.Value, 0, len(n.DHCPSettings.Options))
	for _, o := range n.DHCPSettings.Options {
		ov, d := types.ObjectValue(dhcpOptionAttrTypes, map[string]attr.Value{
			"code":  types.Int64Value(int64(o.Code)),
			"type":  types.Int64Value(int64(o.Type)),
			"value": types.StringValue(o.Value),
		})
		diags.Append(d...)
		elems = append(elems, ov)
	}
	ol, d := types.ListValue(types.ObjectType{AttrTypes: dhcpOptionAttrTypes}, elems)
	diags.Append(d...)
	m.DHCPOptions = ol
	return diags
}

// Create attempts to create a network via the /api/v2 endpoint.
//
// KNOWN LIMITATION: on v6.2 controllers, creating a *new* "interface" (routed)
// network is not supported here — the controller's UI creates networks through
// the official Omada OpenAPI, which needs a separate client-credentials token
// flow. Import, read, update and delete of existing networks all work. See the
// provider README.
func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	fields, diags := r.fieldsFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreateNetwork(ctx, siteID, fields)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create network", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkResourceModel
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
	net, err := r.data.client.GetNetwork(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, net, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	fields, diags := r.fieldsFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateNetwork(ctx, siteID, plan.ID.ValueString(), fields)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update network", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteNetwork(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete network", err.Error())
	}
}

// ImportState accepts "<network_id>" or "<site_name>/<network_id>".
func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
