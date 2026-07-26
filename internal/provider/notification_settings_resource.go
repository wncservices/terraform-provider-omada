// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &notificationSettingsResource{}
	_ resource.ResourceWithConfigure   = &notificationSettingsResource{}
	_ resource.ResourceWithImportState = &notificationSettingsResource{}
)

func NewNotificationSettingsResource() resource.Resource {
	return &notificationSettingsResource{}
}

type notificationSettingsResource struct{ data *providerData }

// notificationToggleModel is the configurable part of one notification.
type notificationToggleModel struct {
	Email   types.Bool `tfsdk:"email"`
	Webhook types.Bool `tfsdk:"webhook"`
	Enable  types.Bool `tfsdk:"enable"`
}

type notificationSettingsModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`

	AlertEmailEnable      types.Bool  `tfsdk:"alert_email_enable"`
	AlertEmailDelayEnable types.Bool  `tfsdk:"alert_email_delay_enable"`
	AlertEmailDelay       types.Int64 `tfsdk:"alert_email_delay"`

	EventEmailEnable      types.Bool  `tfsdk:"event_email_enable"`
	EventEmailDelayEnable types.Bool  `tfsdk:"event_email_delay_enable"`
	EventEmailDelay       types.Int64 `tfsdk:"event_email_delay"`

	WebhookEnable types.Bool `tfsdk:"webhook_enable"`

	Alerts map[string]notificationToggleModel `tfsdk:"alert"`
	Events map[string]notificationToggleModel `tfsdk:"event"`
}

func (r *notificationSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_settings"
}

// toggleAttributes is the nested shape shared by the alert and event maps.
//
// These are Optional and deliberately **not** Computed. Nested Computed
// attributes inside an Optional map are planned as null rather than unknown, so
// filling them from the controller would fail as an inconsistent apply. Leaving
// them Optional-only also matches the intent: you manage the toggles you name,
// and one you do not name is simply not tracked.
func toggleAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"email": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Send this notification by email. Omit to leave the controller's value alone.",
		},
		"webhook": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Send this notification to the webhook. Omit to leave the controller's value alone.",
		},
		"enable": schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: "Whether the notification is active at all. Omit to leave the controller's value alone.",
		},
	}
}

func (r *notificationSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages alert and event notifications: email and webhook delivery, and which " +
			"individual notifications are active.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"The controller knows **131** notifications (63 alerts, 68 events). Rather than making you " +
			"restate all of them, `alert` and `event` are **sparse maps keyed by the controller's " +
			"notification key** (`OSW_DET_STORM`, `DEV_IP_C`, …): name only the ones you care about and " +
			"every other notification keeps whatever the controller has. The descriptive parts of an " +
			"entry — its message, module, severity and applicable device types — are controller-owned " +
			"and not manageable here.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},

			"alert_email_enable":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Deliver alert notifications by email.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"alert_email_delay_enable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Batch alert emails behind a delay.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"alert_email_delay":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Alert email delay (seconds).", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},

			"event_email_enable":       schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Deliver event notifications by email.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"event_email_delay_enable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Batch event emails behind a delay.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"event_email_delay":        schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Event email delay (seconds).", PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}},

			"webhook_enable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Deliver notifications to the configured webhook.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},

			"alert": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Alert notifications to manage, keyed by the controller's notification key.",
				NestedObject:        schema.NestedAttributeObject{Attributes: toggleAttributes()},
			},
			"event": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Event notifications to manage, keyed by the controller's notification key.",
				NestedObject:        schema.NestedAttributeObject{Attributes: toggleAttributes()},
			},
		},
	}
}

func (r *notificationSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *notificationSettingsResource) siteName(m notificationSettingsModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// boolPtr returns a pointer for a set value, or nil so the client leaves the
// controller's value alone.
func boolPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func togglesFrom(m map[string]notificationToggleModel) map[string]omada.NotificationToggle {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]omada.NotificationToggle, len(m))
	for k, t := range m {
		out[k] = omada.NotificationToggle{
			Email:   boolPtr(t.Email),
			Webhook: boolPtr(t.Webhook),
			Enable:  boolPtr(t.Enable),
		}
	}
	return out
}

// groupsFrom collects the top-level sub-object overrides the plan sets.
func (m notificationSettingsModel) groupsFrom() map[string]map[string]any {
	groups := map[string]map[string]any{}
	put := func(group, key string, v any) {
		if groups[group] == nil {
			groups[group] = map[string]any{}
		}
		groups[group][key] = v
	}
	if b := boolPtr(m.AlertEmailEnable); b != nil {
		put("alertEmailSetting", "alertEmailEnable", *b)
	}
	if b := boolPtr(m.AlertEmailDelayEnable); b != nil {
		put("alertEmailSetting", "delayEnable", *b)
	}
	if !m.AlertEmailDelay.IsNull() && !m.AlertEmailDelay.IsUnknown() {
		put("alertEmailSetting", "delay", m.AlertEmailDelay.ValueInt64())
	}
	if b := boolPtr(m.EventEmailEnable); b != nil {
		put("eventEmailSetting", "eventEmailEnable", *b)
	}
	if b := boolPtr(m.EventEmailDelayEnable); b != nil {
		put("eventEmailSetting", "delayEnable", *b)
	}
	if !m.EventEmailDelay.IsNull() && !m.EventEmailDelay.IsUnknown() {
		put("eventEmailSetting", "delay", m.EventEmailDelay.ValueInt64())
	}
	if b := boolPtr(m.WebhookEnable); b != nil {
		put("webhookSetting", "webhookEnable", *b)
	}
	return groups
}

// refresh fills the model from the live document. The alert/event maps are
// refreshed **only for keys already in the model**, which is what keeps a
// sparse configuration from growing to all 131 notifications.
func (m *notificationSettingsModel) refresh(doc map[string]any) {
	getBool := func(group, key string) types.Bool {
		if v, ok := omada.SettingBool(doc, group, key); ok {
			return types.BoolValue(v)
		}
		return types.BoolNull()
	}
	getInt := func(group, key string) types.Int64 {
		if v, ok := omada.SettingInt(doc, group, key); ok {
			return types.Int64Value(v)
		}
		return types.Int64Null()
	}
	m.AlertEmailEnable = getBool("alertEmailSetting", "alertEmailEnable")
	m.AlertEmailDelayEnable = getBool("alertEmailSetting", "delayEnable")
	m.AlertEmailDelay = getInt("alertEmailSetting", "delay")
	m.EventEmailEnable = getBool("eventEmailSetting", "eventEmailEnable")
	m.EventEmailDelayEnable = getBool("eventEmailSetting", "delayEnable")
	m.EventEmailDelay = getInt("eventEmailSetting", "delay")
	m.WebhookEnable = getBool("webhookSetting", "webhookEnable")

	m.Alerts = refreshToggles(m.Alerts, omada.NotificationEntries(doc, "alertNotifications"))
	m.Events = refreshToggles(m.Events, omada.NotificationEntries(doc, "eventNotifications"))
}

// refreshToggles updates the declared keys from the live entries.
//
// Only fields the practitioner actually set are refreshed: an unset toggle
// stays null, because the attributes are Optional-only and Terraform requires
// the applied value to equal the configured one. The effect is that drift is
// detected on exactly the toggles under management. A declared key the
// controller does not know is dropped, so that surfaces as a diff rather than
// a silent no-op.
func refreshToggles(declared map[string]notificationToggleModel, live map[string]map[string]any) map[string]notificationToggleModel {
	if declared == nil {
		return nil
	}
	out := make(map[string]notificationToggleModel, len(declared))
	for key, want := range declared {
		e, ok := live[key]
		if !ok {
			continue
		}
		t := notificationToggleModel{Email: types.BoolNull(), Webhook: types.BoolNull(), Enable: types.BoolNull()}
		if !want.Email.IsNull() {
			if v, ok := e["email"].(bool); ok {
				t.Email = types.BoolValue(v)
			}
		}
		if !want.Webhook.IsNull() {
			if v, ok := e["webhook"].(bool); ok {
				t.Webhook = types.BoolValue(v)
			}
		}
		if !want.Enable.IsNull() {
			if v, ok := e["enable"].(bool); ok {
				t.Enable = types.BoolValue(v)
			}
		}
		out[key] = t
	}
	return out
}

func (r *notificationSettingsResource) write(ctx context.Context, plan *notificationSettingsModel, diags *diagSink) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(*plan))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return
	}
	toggles := map[string]map[string]omada.NotificationToggle{}
	if t := togglesFrom(plan.Alerts); t != nil {
		toggles["alertNotifications"] = t
	}
	if t := togglesFrom(plan.Events); t != nil {
		toggles["eventNotifications"] = t
	}
	if err := r.data.client.UpdateNotificationDoc(ctx, site.ID, omada.NotificationSettingsDoc, plan.groupsFrom(), toggles); err != nil {
		diags.AddError("Unable to update notification settings", err.Error())
		return
	}
	doc, err := r.data.client.GetNotificationDoc(ctx, site.ID, omada.NotificationSettingsDoc)
	if err != nil {
		diags.AddError("Unable to read notification settings", err.Error())
		return
	}
	plan.refresh(doc)
	plan.ID = types.StringValue(site.ID)
	plan.SiteID = types.StringValue(site.ID)
	plan.Site = types.StringValue(site.Name)
}

func (r *notificationSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := state.SiteID.ValueString()
	if siteID == "" {
		site, err := r.data.client.ResolveSite(ctx, r.siteName(state))
		if err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
		siteID = site.ID
		state.Site = types.StringValue(site.Name)
	}
	doc, err := r.data.client.GetNotificationDoc(ctx, siteID, omada.NotificationSettingsDoc)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read notification settings", err.Error())
		return
	}
	state.refresh(doc)
	state.ID = types.StringValue(siteID)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete is a no-op: notification settings always exist and are deliberately
// not reset when the resource leaves the configuration.
func (r *notificationSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name.
func (r *notificationSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			"expected the site name, e.g. `terraform import omada_notification_settings.this Home`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
