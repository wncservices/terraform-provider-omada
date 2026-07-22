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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &firewallACLResource{}
	_ resource.ResourceWithConfigure   = &firewallACLResource{}
	_ resource.ResourceWithImportState = &firewallACLResource{}
)

func NewFirewallACLResource() resource.Resource {
	return &firewallACLResource{}
}

type firewallACLResource struct {
	data *providerData
}

type firewallACLModel struct {
	ID              types.String `tfsdk:"id"`
	Site            types.String `tfsdk:"site"`
	SiteID          types.String `tfsdk:"site_id"`
	Type            types.String `tfsdk:"type"`
	Name            types.String `tfsdk:"name"`
	Enable          types.Bool   `tfsdk:"enable"`
	Policy          types.String `tfsdk:"policy"`
	Protocols       types.List   `tfsdk:"protocols"`
	SourceType      types.Int64  `tfsdk:"source_type"`
	SourceIDs       types.List   `tfsdk:"source_ids"`
	DestinationType types.Int64  `tfsdk:"destination_type"`
	DestinationIDs  types.List   `tfsdk:"destination_ids"`
	Direction       types.Object `tfsdk:"direction"`
}

type aclDirectionModel struct {
	LanToWan types.Bool `tfsdk:"lan_to_wan"`
	LanToLan types.Bool `tfsdk:"lan_to_lan"`
	WanInIDs types.List `tfsdk:"wan_in_ids"`
	VpnInIDs types.List `tfsdk:"vpn_in_ids"`
}

var directionAttrTypes = map[string]attr.Type{
	"lan_to_wan": types.BoolType,
	"lan_to_lan": types.BoolType,
	"wan_in_ids": types.ListType{ElemType: types.StringType},
	"vpn_in_ids": types.ListType{ElemType: types.StringType},
}

var aclTypeToInt = map[string]int{"gateway": omada.ACLTypeGateway, "switch": omada.ACLTypeSwitch, "eap": omada.ACLTypeEAP}
var aclTypeToStr = map[int]string{omada.ACLTypeGateway: "gateway", omada.ACLTypeSwitch: "switch", omada.ACLTypeEAP: "eap"}
var policyToInt = map[string]int{"deny": 0, "permit": 1}
var policyToStr = map[int]string{0: "deny", 1: "permit"}

func (r *firewallACLResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_acl"
}

func (r *firewallACLResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a firewall ACL rule on the Omada controller. Sources/destinations reference network IDs or IP-group IDs (per `source_type`/`destination_type`).",
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
			"type": schema.StringAttribute{
				MarkdownDescription: "ACL type: `gateway`, `switch`, or `eap`. Changing forces replacement.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("gateway"),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
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
			"policy": schema.StringAttribute{
				MarkdownDescription: "`permit` or `deny`.",
				Required:            true,
			},
			"protocols": schema.ListAttribute{
				MarkdownDescription: "IP protocol numbers (6=TCP, 17=UDP, 1=ICMP, 256=all). Defaults to all.",
				ElementType:         types.Int64Type,
				Optional:            true,
				Computed:            true,
			},
			"source_type": schema.Int64Attribute{
				MarkdownDescription: "Source entity type: 0=network, 1=IP group.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"source_ids": schema.ListAttribute{
				MarkdownDescription: "Source entity IDs (network or IP-group IDs, per source_type).",
				ElementType:         types.StringType,
				Required:            true,
			},
			"destination_type": schema.Int64Attribute{
				MarkdownDescription: "Destination entity type: 0=network, 1=IP group.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
			},
			"destination_ids": schema.ListAttribute{
				MarkdownDescription: "Destination entity IDs.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"direction": schema.SingleNestedAttribute{
				MarkdownDescription: "Gateway ACL direction. Defaults to LAN-to-LAN.",
				Optional:            true,
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"lan_to_wan": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false)},
					"lan_to_lan": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
					"wan_in_ids": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true},
					"vpn_in_ids": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true},
				},
			},
		},
	}
}

func (r *firewallACLResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *firewallACLResource) siteName(m firewallACLModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *firewallACLResource) inputFrom(ctx context.Context, m firewallACLModel) (*omada.ACLInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	aclType, ok := aclTypeToInt[strings.ToLower(m.Type.ValueString())]
	if !ok {
		diags.AddAttributeError(path.Root("type"), "Invalid type", "type must be gateway, switch, or eap")
	}
	policy, ok := policyToInt[strings.ToLower(m.Policy.ValueString())]
	if !ok {
		diags.AddAttributeError(path.Root("policy"), "Invalid policy", "policy must be permit or deny")
	}

	protocols, d := intSlice(ctx, m.Protocols)
	diags.Append(d...)
	if len(protocols) == 0 {
		protocols = []int{256} // all
	}
	srcIDs, d := stringSlice(ctx, m.SourceIDs)
	diags.Append(d...)
	dstIDs, d := stringSlice(ctx, m.DestinationIDs)
	diags.Append(d...)

	dir := omada.ACLDirection{LanToLan: true, WanInIDs: []string{}, VpnInIDs: []string{}}
	if !m.Direction.IsNull() && !m.Direction.IsUnknown() {
		var dm aclDirectionModel
		diags.Append(m.Direction.As(ctx, &dm, basetypes.ObjectAsOptions{UnhandledNullAsEmpty: true, UnhandledUnknownAsEmpty: true})...)
		dir.LanToWan = dm.LanToWan.ValueBool()
		dir.LanToLan = dm.LanToLan.ValueBool()
		if wan, dd := stringSlice(ctx, dm.WanInIDs); dd == nil {
			dir.WanInIDs = nilToEmpty(wan)
		}
		if vpn, dd := stringSlice(ctx, dm.VpnInIDs); dd == nil {
			dir.VpnInIDs = nilToEmpty(vpn)
		}
	}
	if diags.HasError() {
		return nil, diags
	}
	return &omada.ACLInput{
		Type: aclType, Name: m.Name.ValueString(), Status: m.Enable.ValueBool(), Policy: policy,
		Protocols: protocols, SourceType: int(m.SourceType.ValueInt64()), SourceIDs: srcIDs,
		DestinationType: int(m.DestinationType.ValueInt64()), DestinationIDs: dstIDs, Direction: dir,
	}, diags
}

func (r *firewallACLResource) apply(ctx context.Context, a *omada.ACL, m *firewallACLModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(a.ID)
	m.Name = types.StringValue(a.Name)
	m.Enable = types.BoolValue(a.Status)
	if s, ok := aclTypeToStr[a.Type]; ok {
		m.Type = types.StringValue(s)
	}
	if s, ok := policyToStr[a.Policy]; ok {
		m.Policy = types.StringValue(s)
	}
	m.SourceType = types.Int64Value(int64(a.SourceType))
	m.DestinationType = types.Int64Value(int64(a.DestinationType))

	protos, d := intListValue(ctx, a.Protocols)
	diags.Append(d...)
	m.Protocols = protos
	src, d := stringListValue(ctx, a.SourceIDs)
	diags.Append(d...)
	m.SourceIDs = src
	dst, d := stringListValue(ctx, a.DestinationIDs)
	diags.Append(d...)
	m.DestinationIDs = dst

	wan, d := stringListValue(ctx, a.Direction.WanInIDs)
	diags.Append(d...)
	vpn, d := stringListValue(ctx, a.Direction.VpnInIDs)
	diags.Append(d...)
	dir, d := types.ObjectValue(directionAttrTypes, map[string]attr.Value{
		"lan_to_wan": types.BoolValue(a.Direction.LanToWan),
		"lan_to_lan": types.BoolValue(a.Direction.LanToLan),
		"wan_in_ids": wan,
		"vpn_in_ids": vpn,
	})
	diags.Append(d...)
	m.Direction = dir
	return diags
}

func (r *firewallACLResource) aclType(m firewallACLModel) int {
	return aclTypeToInt[strings.ToLower(m.Type.ValueString())]
}

func (r *firewallACLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallACLModel
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
	created, err := r.data.client.CreateACL(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create ACL", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallACLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallACLModel
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
	// On import only the id is known, so the ACL type isn't in state yet. Search
	// every type for the id rather than assuming `gateway`; apply() then records
	// the type it was actually found under.
	var a *omada.ACL
	var err error
	if state.Type.IsNull() || state.Type.IsUnknown() || state.Type.ValueString() == "" {
		for _, t := range []int{omada.ACLTypeGateway, omada.ACLTypeSwitch, omada.ACLTypeEAP} {
			if found, ferr := r.data.client.GetACL(ctx, siteID, t, state.ID.ValueString()); ferr == nil {
				a, err = found, nil
				break
			}
			err = fmt.Errorf("acl %q not found in any type", state.ID.ValueString())
		}
	} else {
		a, err = r.data.client.GetACL(ctx, siteID, r.aclType(state), state.ID.ValueString())
	}
	if err != nil || a == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, a, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *firewallACLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallACLModel
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
	updated, err := r.data.client.UpdateACL(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update ACL", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firewallACLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallACLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteACL(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete ACL", err.Error())
	}
}

// ImportState accepts "<id>" or "<site_name>/<id>".
func (r *firewallACLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
