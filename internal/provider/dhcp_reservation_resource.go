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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &dhcpReservationResource{}
	_ resource.ResourceWithConfigure   = &dhcpReservationResource{}
	_ resource.ResourceWithImportState = &dhcpReservationResource{}
)

func NewDHCPReservationResource() resource.Resource { return &dhcpReservationResource{} }

type dhcpReservationResource struct{ data *providerData }

type dhcpReservationResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Site      types.String `tfsdk:"site"`
	SiteID    types.String `tfsdk:"site_id"`
	NetworkID types.String `tfsdk:"network_id"`
	MAC       macValue     `tfsdk:"mac"`
	IP        types.String `tfsdk:"ip"`
	Name      types.String `tfsdk:"name"`
	Enable    types.Bool   `tfsdk:"enable"`

	NetworkName   types.String `tfsdk:"network_name"`
	ExportToIPMac types.Bool   `tfsdk:"export_to_ip_mac_binding"`
}

func (r *dhcpReservationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_reservation"
}

func (r *dhcpReservationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a DHCP reservation: pins a client to a fixed address.\n\n" +
			"The **MAC address is this object's key** on the controller, not its id — so changing " +
			"`mac` replaces the reservation. Any separator (`:`, `-`, `.`) and either case is " +
			"accepted and compared equivalently, so `aa:bb:cc:dd:ee:ff` and `AA-BB-CC-DD-EE-FF` " +
			"refer to the same device and neither produces a permanent diff.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"network_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "ID of the LAN network the reservation belongs to (see the `omada_networks` data source).",
			},
			"mac": schema.StringAttribute{
				// Compares by MAC, not by text — see mac_type.go for why this
				// needs a custom type rather than normalisation.
				CustomType: macType{},
				Required:   true,
				MarkdownDescription: "Client MAC address. Accepts `:`, `-` or `.` separators in any case. " +
					"Changing it forces replacement, because the controller keys the object on the MAC.",
				// RequiresReplaceIf, not RequiresReplace: the latter compares
				// raw strings, so re-spelling an address (or importing, which
				// yields the controller's spelling) would plan a destroy and
				// recreate of a working reservation over pure formatting.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(macChanged,
						"Replaces the reservation when the MAC actually changes.",
						"Replaces the reservation when the MAC actually changes."),
				},
			},
			"ip":   schema.StringAttribute{Required: true, MarkdownDescription: "Address to reserve. Must sit inside the network's subnet."},
			"name": schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Display name for the reservation."},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether the reservation is active.",
			},
			"network_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the network, as reported by the controller."},
			"export_to_ip_mac_binding": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the reservation is also exported as an IP-MAC binding. " +
					"**Read-only**: the controller forces this on regardless of what is sent, so it is " +
					"reported rather than managed.",
			},
		},
	}
}

func (r *dhcpReservationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dhcpReservationResource) siteName(m dhcpReservationResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (m dhcpReservationResourceModel) toAPI() omada.DHCPReservation {
	return omada.DHCPReservation{
		NetID:  m.NetworkID.ValueString(),
		MAC:    omada.NormalizeMAC(m.MAC.ValueString()),
		IP:     m.IP.ValueString(),
		Name:   m.Name.ValueString(),
		Status: m.Enable.ValueBool(),
	}
}

func (m *dhcpReservationResourceModel) refresh(d *omada.DHCPReservation) {
	m.ID = types.StringValue(d.ID)
	m.NetworkID = types.StringValue(d.NetID)
	// Keep the spelling already in the model when it denotes the same address.
	// Terraform requires an applied value to equal its planned one, so the
	// configured rendering has to survive; macChanged makes sure the differing
	// rendering is never mistaken for a different device.
	if omada.NormalizeMAC(m.MAC.ValueString()) != d.MAC {
		m.MAC = newMACValue(d.MAC)
	}
	m.IP = types.StringValue(d.IP)
	m.Name = types.StringValue(d.Name)
	m.Enable = types.BoolValue(d.Status)
	m.NetworkName = types.StringValue(d.NetName)
	m.ExportToIPMac = types.BoolValue(d.ExportToIPMacBinding)
}

func (r *dhcpReservationResource) resolveSite(ctx context.Context, m dhcpReservationResourceModel, diags *diagSink) string {
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

func (r *dhcpReservationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dhcpReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.CreateDHCPReservation(ctx, siteID, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create dhcp reservation", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dhcpReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetDHCPReservationByMAC(ctx, siteID, state.MAC.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dhcpReservationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dhcpReservationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	// Key on the MAC already stored: `mac` forces replacement, so it cannot
	// have changed here, but keying on state keeps that explicit.
	if err := r.data.client.UpdateDHCPReservation(ctx, siteID, state.MAC.ValueString(), plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Unable to update dhcp reservation", err.Error())
		return
	}
	cur, err := r.data.client.GetDHCPReservationByMAC(ctx, siteID, plan.MAC.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read dhcp reservation after update", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dhcpReservationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dhcpReservationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteDHCPReservation(ctx, siteID, state.MAC.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete dhcp reservation", err.Error())
		return
	}
	// The controller answers 0 even for a key that matched nothing, so confirm
	// the object is actually gone rather than trusting the status code.
	if _, err := r.data.client.GetDHCPReservationByMAC(ctx, siteID, state.MAC.ValueString()); err == nil {
		resp.Diagnostics.AddError(
			"DHCP reservation still present after delete",
			fmt.Sprintf("the controller accepted the delete for %s but the reservation is still listed; "+
				"it may be pinned by an IP-MAC binding", state.MAC.ValueString()),
		)
	}
}

// ImportState takes the MAC address, or "<site>/<mac>" — the MAC, not the id,
// because that is what the controller keys the object on.
func (r *dhcpReservationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), newMACValue(omada.NormalizeMAC(id)))...)
}

// macChanged reports whether the MAC genuinely differs, ignoring separator and
// case. Used to decide replacement.
func macChanged(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	resp.RequiresReplace = omada.NormalizeMAC(req.StateValue.ValueString()) !=
		omada.NormalizeMAC(req.PlanValue.ValueString())
}
