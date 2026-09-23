// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &controllerSettingsResource{}
	_ resource.ResourceWithConfigure   = &controllerSettingsResource{}
	_ resource.ResourceWithImportState = &controllerSettingsResource{}
)

func NewControllerSettingsResource() resource.Resource { return &controllerSettingsResource{} }

// controllerSettingsResource is the provider's first CONTROLLER-scoped
// resource. It deliberately has no `site` or `site_id` attribute: the document
// it manages lives above sites, and §2.7a's Optional-and-Computed `site` rule
// is about site-scoped resources rather than a requirement on every resource.
//
// Verified end to end against a live v6.2.14.11 controller: import with an
// empty resource body, then a clean no-op plan (so nothing here reports false
// drift), then a real change to `general.name` — re-imported afterwards to
// confirm the controller had actually applied it rather than merely accepting
// it (§5.5a), with the rest of that section and every other section unchanged.
type controllerSettingsResource struct{ data *providerData }

type controllerSettingsResourceModel struct {
	ID types.String `tfsdk:"id"`

	Name       types.String `tfsdk:"name"`
	TimeZone   types.String `tfsdk:"time_zone"`
	Region     types.String `tfsdk:"region"`
	NTPEnable  types.Bool   `tfsdk:"ntp_enable"`
	NTPServers types.List   `tfsdk:"ntp_servers"`

	HostName            types.String `tfsdk:"host_name"`
	ManageHTTPPort      types.Int64  `tfsdk:"manage_http_port"`
	ManageHTTPSPort     types.Int64  `tfsdk:"manage_https_port"`
	PortalHTTPPort      types.Int64  `tfsdk:"portal_http_port"`
	PortalHTTPSPort     types.Int64  `tfsdk:"portal_https_port"`
	PortalHTTPSRedirect types.Bool   `tfsdk:"portal_https_redirect"`

	DeviceHostEnable types.Bool   `tfsdk:"device_host_enable"`
	DeviceHost       types.String `tfsdk:"device_host"`

	WebControlHTTP  types.Bool `tfsdk:"device_web_control_http"`
	WebControlHTTPS types.Bool `tfsdk:"device_web_control_https"`
	AppDiscovery    types.Bool `tfsdk:"app_discovery"`

	ControllerNotification types.Bool `tfsdk:"firmware_notification"`

	LogLevelType types.String `tfsdk:"log_level_type"`
}

func (r *controllerSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_controller_settings"
}

func (r *controllerSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages controller-wide settings (Global Settings → Controller Settings).\n\n" +
			"~> **This resource is controller-scoped and has no `site`.** It is a singleton: there is " +
			"one such document per controller, so declare at most one of these.\n\n" +
			"~> **`device_host` is the setting most worth managing here.** It is the address adopted " +
			"devices are told to contact. Left to the controller, it is inferred from the host's " +
			"interfaces and can land on one nothing can reach — a VPN tunnel, say — after which " +
			"devices drop off some time later and look like flaky hardware rather than a " +
			"misconfiguration.\n\n" +
			"Every attribute is optional and only what you set is sent. Anything you leave out keeps " +
			"its current value and will never report drift. `terraform destroy` forgets the settings " +
			"without changing the controller.\n\n" +
			"SMTP, certificates and the RADIUS blocks are deliberately not managed here: they carry " +
			"credentials and key material that belong in their own resources.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The controller's omadac id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Controller name, shown in the UI and in its notifications.",
			},
			"time_zone": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "IANA time zone, e.g. `America/Los_Angeles`. Drives log timestamps " +
					"and anything scheduled.",
			},
			"region": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Regulatory region, e.g. `United States`.\n\n" +
					"~> This constrains the channels and transmit power the radios may legally use. " +
					"Setting it to somewhere you are not is not a way to get more power.",
			},
			"ntp_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the controller uses its own NTP servers rather than the " +
					"host clock.",
			},
			"ntp_servers": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				MarkdownDescription: "NTP servers, used when `ntp_enable` is set.",
			},

			"host_name": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The address the controller believes it is reachable at, used for " +
					"its own links and portal URLs. See also `device_host`.",
			},
			"manage_http_port": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Management HTTP port. Changing it moves the UI.",
			},
			"manage_https_port": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Management HTTPS port. Changing it moves the UI, and adopted " +
					"devices reconnect on the new one.",
			},
			"portal_http_port": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Captive-portal HTTP port.",
			},
			"portal_https_port": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Captive-portal HTTPS port.",
			},
			"portal_https_redirect": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Redirect portal HTTP to HTTPS.",
			},

			"device_host_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Tell devices an explicit address instead of one the controller " +
					"infers. Set this with `device_host`; on its own it is meaningless.",
			},
			"device_host": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The address adopted devices are told to contact.\n\n" +
					"~> Must be reachable **from the devices**, which is not always the address you " +
					"reach the controller on. A controller on a host with several interfaces will " +
					"otherwise advertise whichever one it inferred.",
			},

			"device_web_control_http": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Allow reaching an adopted device's own web UI over plain HTTP.\n\n" +
					"~> That UI takes a password over an unencrypted connection.",
			},
			"device_web_control_https": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Allow reaching an adopted device's own web UI over HTTPS.",
			},
			"app_discovery": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Answer discovery probes from the Omada mobile app on the local network.",
			},

			"firmware_notification": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Notify when a controller firmware update is available.",
			},

			"log_level_type": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Logging level mode, e.g. `AUTO`. The per-category levels are read " +
					"and preserved but not individually settable here.",
			},
		},
	}
}

func (r *controllerSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// changed builds the PATCH body a whole section at a time.
//
// Sections differ in whether they accept a partial sub-object — see the note on
// omada.ControllerSettings — so each is read, modified and sent complete. A
// section none of whose attributes the practitioner set is omitted entirely, so
// an apply that changes nothing writes nothing.
func (r *controllerSettingsResource) changed(ctx context.Context, plan controllerSettingsResourceModel, cur *omada.ControllerSettings, diags *diag.Diagnostics) map[string]any {
	out := map[string]any{}

	if known(plan.Name) || known(plan.TimeZone) || known(plan.Region) ||
		known(plan.NTPEnable) || known(plan.NTPServers) {
		g := omada.ControllerGeneral{}
		if cur.General != nil {
			g = *cur.General
		}
		if known(plan.Name) {
			g.Name = plan.Name.ValueString()
		}
		if known(plan.TimeZone) {
			g.TimeZone = plan.TimeZone.ValueString()
		}
		if known(plan.Region) {
			g.Region = plan.Region.ValueString()
		}
		if known(plan.NTPEnable) {
			g.NTPEnable = plan.NTPEnable.ValueBool()
		}
		if known(plan.NTPServers) {
			servers, d := stringSlice(ctx, plan.NTPServers)
			diags.Append(d...)
			g.NTPServers = servers
		}
		g.NTPServers = nilToEmpty(g.NTPServers)
		out["general"] = g
	}

	if known(plan.HostName) || known(plan.ManageHTTPPort) || known(plan.ManageHTTPSPort) ||
		known(plan.PortalHTTPPort) || known(plan.PortalHTTPSPort) || known(plan.PortalHTTPSRedirect) {
		w := omada.ControllerWebPort{}
		if cur.WebPort != nil {
			w = *cur.WebPort
		}
		if known(plan.HostName) {
			w.HostName = plan.HostName.ValueString()
		}
		if known(plan.ManageHTTPPort) {
			w.ManageHTTPPort = int(plan.ManageHTTPPort.ValueInt64())
		}
		if known(plan.ManageHTTPSPort) {
			w.ManageHTTPSPort = int(plan.ManageHTTPSPort.ValueInt64())
		}
		if known(plan.PortalHTTPPort) {
			w.PortalHTTPPort = int(plan.PortalHTTPPort.ValueInt64())
		}
		if known(plan.PortalHTTPSPort) {
			w.PortalHTTPSPort = int(plan.PortalHTTPSPort.ValueInt64())
		}
		if known(plan.PortalHTTPSRedirect) {
			w.PortalHTTPSRedirect = plan.PortalHTTPSRedirect.ValueBool()
		}
		out["webPort"] = w
	}

	if known(plan.DeviceHostEnable) || known(plan.DeviceHost) {
		d := omada.ControllerDeviceManage{}
		if cur.DeviceManage != nil {
			d = *cur.DeviceManage
		}
		if known(plan.DeviceHostEnable) {
			d.DeviceHostEnable = plan.DeviceHostEnable.ValueBool()
		}
		if known(plan.DeviceHost) {
			d.DeviceHost = plan.DeviceHost.ValueString()
		}
		out["deviceManage"] = d
	}

	if known(plan.WebControlHTTP) || known(plan.WebControlHTTPS) || known(plan.AppDiscovery) {
		a := omada.ControllerDeviceAccess{}
		if cur.DeviceAccessManagement != nil {
			a = *cur.DeviceAccessManagement
		}
		if known(plan.WebControlHTTP) {
			a.WebControlHTTP = plan.WebControlHTTP.ValueBool()
		}
		if known(plan.WebControlHTTPS) {
			a.WebControlHTTPS = plan.WebControlHTTPS.ValueBool()
		}
		if known(plan.AppDiscovery) {
			a.AppDiscovery = plan.AppDiscovery.ValueBool()
		}
		out["deviceAccessManagement"] = a
	}

	if known(plan.ControllerNotification) {
		out["firmware"] = omada.ControllerFirmware{
			ControllerNotification: plan.ControllerNotification.ValueBool(),
		}
	}

	if known(plan.LogLevelType) {
		l := omada.ControllerLoggingLevel{}
		if cur.LoggingLevel != nil {
			l = *cur.LoggingLevel
		}
		l.Type = plan.LogLevelType.ValueString()
		out["loggingLevel"] = l
	}

	return out
}

// refresh writes the controller's document into the model.
//
// Every attribute is Optional+Computed and every one is filled in from the
// read, so a practitioner who sets three of them still gets the other
// twenty-odd in state, and `terraform show` describes the controller rather
// than just the parts under management.
func (r *controllerSettingsResource) refresh(ctx context.Context, cs *omada.ControllerSettings, m *controllerSettingsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(r.data.client.OmadacID())

	if g := cs.General; g != nil {
		m.Name = types.StringValue(g.Name)
		m.TimeZone = types.StringValue(g.TimeZone)
		m.Region = types.StringValue(g.Region)
		m.NTPEnable = types.BoolValue(g.NTPEnable)
		servers, d := stringListValue(ctx, g.NTPServers)
		diags.Append(d...)
		m.NTPServers = servers
	}

	if w := cs.WebPort; w != nil {
		m.HostName = types.StringValue(w.HostName)
		m.ManageHTTPPort = types.Int64Value(int64(w.ManageHTTPPort))
		m.ManageHTTPSPort = types.Int64Value(int64(w.ManageHTTPSPort))
		m.PortalHTTPPort = types.Int64Value(int64(w.PortalHTTPPort))
		m.PortalHTTPSPort = types.Int64Value(int64(w.PortalHTTPSPort))
		m.PortalHTTPSRedirect = types.BoolValue(w.PortalHTTPSRedirect)
	}

	if d := cs.DeviceManage; d != nil {
		m.DeviceHostEnable = types.BoolValue(d.DeviceHostEnable)
		m.DeviceHost = types.StringValue(d.DeviceHost)
	}

	if a := cs.DeviceAccessManagement; a != nil {
		m.WebControlHTTP = types.BoolValue(a.WebControlHTTP)
		m.WebControlHTTPS = types.BoolValue(a.WebControlHTTPS)
		m.AppDiscovery = types.BoolValue(a.AppDiscovery)
	}

	if f := cs.Firmware; f != nil {
		m.ControllerNotification = types.BoolValue(f.ControllerNotification)
	}

	if l := cs.LoggingLevel; l != nil {
		m.LogLevelType = types.StringValue(l.Type)
	}

	return diags
}

// apply is the whole of Create and Update: read the current document, send the
// sections the plan changes, read it back.
//
// Create and Update are identical because the document always exists. "Create"
// here means "start managing", which is why nothing checks whether settings
// are already present.
func (r *controllerSettingsResource) apply(ctx context.Context, plan *controllerSettingsResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	cur, err := r.data.client.GetControllerSettings(ctx)
	if err != nil {
		diags.AddError("Unable to read controller settings", err.Error())
		return diags
	}

	body := r.changed(ctx, *plan, cur, &diags)
	if diags.HasError() {
		return diags
	}
	if len(body) > 0 {
		if err := r.data.client.UpdateControllerSettings(ctx, body); err != nil {
			diags.AddError("Unable to update controller settings", err.Error())
			return diags
		}
		// Re-read rather than assume the plan took: the controller normalises
		// some of these (a host name may come back trimmed or lowercased) and
		// state should hold what it actually stored.
		if cur, err = r.data.client.GetControllerSettings(ctx); err != nil {
			diags.AddError("Unable to read controller settings after update", err.Error())
			return diags
		}
	}

	diags.Append(r.refresh(ctx, cur, plan)...)
	return diags
}

func (r *controllerSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan controllerSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *controllerSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state controllerSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A failed read is an error, never a missing resource. The document cannot
	// be deleted, so RemoveResource here would turn an unreachable controller
	// into a plan that recreates settings.
	cur, err := r.data.client.GetControllerSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read controller settings", err.Error())
		return
	}

	resp.Diagnostics.Append(r.refresh(ctx, cur, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *controllerSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan controllerSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete forgets the settings without changing the controller.
func (r *controllerSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Controller settings left as configured",
		"Controller settings cannot be deleted, so Terraform has only stopped managing them. They "+
			"are unchanged. Resetting them on destroy would mean this provider guessing at defaults "+
			"for the controller every adopted device depends on.",
	)
}

// ImportState ignores the import id. The document is a singleton, so there is
// nothing to address; `terraform import ... controller` reads fine and any
// other string works too.
func (r *controllerSettingsResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	cur, err := r.data.client.GetControllerSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read controller settings", err.Error())
		return
	}
	var m controllerSettingsResourceModel
	resp.Diagnostics.Append(r.refresh(ctx, cur, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
