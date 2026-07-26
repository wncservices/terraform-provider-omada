// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// A flat singleton settings document (see internal/omada/settings.go) maps onto
// Terraform almost mechanically: every controller key becomes one attribute,
// there is nothing to create or destroy, and the site is the identity. Rather
// than copy that boilerplate per endpoint, this file implements it once and
// drives it from a small table; a new singleton resource is then just a spec.
//
// The resource uses path-addressed plan/state access rather than a struct with
// `tfsdk` tags, because the field list differs per spec and one Go type cannot
// cover them all.
//
// Every field is Optional + Computed with no default, per the "null is not
// false" invariant in DESIGN.md §2.6: a field the practitioner never sets is
// left alone on the controller and reflected back from the live document.

type settingKind int

const (
	kindBool settingKind = iota
	kindInt
	kindIntList
	// kindIntListRO is reference data the controller owns: reported as a
	// Computed attribute and never written back. The IPS endpoint returns the
	// category set each protection level covers alongside the real settings,
	// and those lists are descriptive, not configuration.
	kindIntListRO
	// kindString is an ordinary string setting.
	kindString
	// kindStringList is a list of strings, e.g. a set of object ids.
	kindStringList
	// kindStringWO is a secret: a Terraform write-only attribute. The value is
	// supplied on apply and never persisted to state or plan, because the
	// controller returns secrets like the SNMP v3 password in plaintext on
	// read (DESIGN.md §2.6). Values come from the configuration, not the plan.
	kindStringWO
)

// readOnly reports whether a kind is controller-owned and must not be sent.
func (k settingKind) readOnly() bool { return k == kindIntListRO }

// writeOnly reports whether a kind is a secret that must never reach state.
func (k settingKind) writeOnly() bool { return k == kindStringWO }

// settingField maps one controller JSON key to one Terraform attribute.
type settingField struct {
	attr string
	key  string
	kind settingKind
	desc string
}

// settingsSpec fully describes a singleton settings resource.
type settingsSpec struct {
	typeName string // without the provider prefix, e.g. "alg"
	doc      omada.SettingDoc
	desc     string
	fields   []settingField
}

// attrGetter / attrSetter are the small slices of tfsdk.Plan and tfsdk.State
// this resource needs, so the same code can read a plan and write a state.
type attrGetter interface {
	GetAttribute(ctx context.Context, p path.Path, target any) diag.Diagnostics
}

type attrSetter interface {
	SetAttribute(ctx context.Context, p path.Path, val any) diag.Diagnostics
}

type settingsResource struct {
	data *providerData
	spec settingsSpec
}

var (
	_ resource.Resource                = &settingsResource{}
	_ resource.ResourceWithConfigure   = &settingsResource{}
	_ resource.ResourceWithImportState = &settingsResource{}
)

func newSettingsResource(spec settingsSpec) resource.Resource {
	return &settingsResource{spec: spec}
}

func (r *settingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.typeName
}

func (r *settingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *settingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The site ID.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		// Computed as well as Optional, and sticky across plans: import records
		// the site name it was given, so a configuration that omits `site`
		// (relying on the provider default) does not then see it flip to null
		// and force a needless replacement.
		"site": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
				stringplanmodifier.RequiresReplace(),
			},
		},
	}
	for _, f := range r.spec.fields {
		switch f.kind {
		case kindBool:
			attrs[f.attr] = schema.BoolAttribute{
				Optional: true, Computed: true, MarkdownDescription: f.desc,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			}
		case kindInt:
			attrs[f.attr] = schema.Int64Attribute{
				Optional: true, Computed: true, MarkdownDescription: f.desc,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			}
		case kindString:
			attrs[f.attr] = schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: f.desc,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			}
		case kindStringWO:
			// Not Computed: WriteOnly forbids it, and there is nothing to
			// compute — the value is never stored.
			attrs[f.attr] = schema.StringAttribute{
				Optional: true, Sensitive: true, WriteOnly: true, MarkdownDescription: f.desc,
			}
		case kindStringList:
			attrs[f.attr] = schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true, Computed: true, MarkdownDescription: f.desc,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			}
		case kindIntList, kindIntListRO:
			attrs[f.attr] = schema.ListAttribute{
				ElementType: types.Int64Type,
				Optional:    !f.kind.readOnly(), Computed: true, MarkdownDescription: f.desc,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			}
		}
	}
	resp.Schema = schema.Schema{MarkdownDescription: r.spec.desc, Attributes: attrs}
}

// siteName resolves the configured site, falling back to the provider default.
func (r *settingsResource) siteName(ctx context.Context, src attrGetter) string {
	var site types.String
	if d := src.GetAttribute(ctx, path.Root("site"), &site); d.HasError() {
		return r.data.defaultSite
	}
	if !site.IsNull() && site.ValueString() != "" {
		return site.ValueString()
	}
	return r.data.defaultSite
}

// fieldsFrom collects the attributes the practitioner actually set. Null and
// unknown attributes are skipped so unmanaged keys are never written.
func (r *settingsResource) fieldsFrom(ctx context.Context, src, cfg attrGetter, diags *diag.Diagnostics) map[string]any {
	out := map[string]any{}
	for _, f := range r.spec.fields {
		if f.kind.readOnly() {
			continue // controller-owned; reported, never written
		}
		// Write-only values exist only in the configuration.
		if f.kind.writeOnly() {
			var v types.String
			diags.Append(cfg.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
				out[f.key] = v.ValueString()
			}
			continue
		}
		switch f.kind {
		case kindString:
			var v types.String
			diags.Append(src.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() {
				out[f.key] = v.ValueString()
			}
		case kindBool:
			var v types.Bool
			diags.Append(src.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() {
				out[f.key] = v.ValueBool()
			}
		case kindInt:
			var v types.Int64
			diags.Append(src.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() {
				out[f.key] = v.ValueInt64()
			}
		case kindStringList:
			var v types.List
			diags.Append(src.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() {
				vals, d := stringSlice(ctx, v)
				diags.Append(d...)
				if diags.HasError() {
					return nil
				}
				out[f.key] = nilToEmpty(vals)
			}
		case kindIntList:
			var v types.List
			diags.Append(src.GetAttribute(ctx, path.Root(f.attr), &v)...)
			if diags.HasError() {
				return nil
			}
			if !v.IsNull() && !v.IsUnknown() {
				vals, d := intSlice(ctx, v)
				diags.Append(d...)
				if diags.HasError() {
					return nil
				}
				out[f.key] = vals
			}
		}
	}
	return out
}

// refresh writes every modelled key from the live document into state. This
// also resolves the unknowns left by Optional+Computed attributes that the
// practitioner did not set.
func (r *settingsResource) refresh(ctx context.Context, doc map[string]any, dst attrSetter, diags *diag.Diagnostics) {
	for _, f := range r.spec.fields {
		if f.kind.writeOnly() {
			continue // secret: must stay null in state
		}
		raw, present := doc[f.key]
		switch f.kind {
		case kindString:
			v := types.StringNull()
			if str, ok := raw.(string); ok && present {
				v = types.StringValue(str)
			}
			diags.Append(dst.SetAttribute(ctx, path.Root(f.attr), v)...)
		case kindBool:
			v := types.BoolNull()
			if b, ok := raw.(bool); ok && present {
				v = types.BoolValue(b)
			}
			diags.Append(dst.SetAttribute(ctx, path.Root(f.attr), v)...)
		case kindInt:
			v := types.Int64Null()
			if n, ok := raw.(float64); ok && present {
				v = types.Int64Value(int64(n))
			}
			diags.Append(dst.SetAttribute(ctx, path.Root(f.attr), v)...)
		case kindStringList:
			items, _ := raw.([]any)
			vals := make([]string, 0, len(items))
			for _, it := range items {
				if str, ok := it.(string); ok {
					vals = append(vals, str)
				}
			}
			lv, d := stringListValue(ctx, vals)
			diags.Append(d...)
			if diags.HasError() {
				return
			}
			diags.Append(dst.SetAttribute(ctx, path.Root(f.attr), lv)...)
		case kindIntList, kindIntListRO:
			items, _ := raw.([]any)
			vals := make([]int, 0, len(items))
			for _, it := range items {
				if n, ok := it.(float64); ok {
					vals = append(vals, int(n))
				}
			}
			lv, d := intListValue(ctx, vals)
			diags.Append(d...)
			if diags.HasError() {
				return
			}
			diags.Append(dst.SetAttribute(ctx, path.Root(f.attr), lv)...)
		}
		if diags.HasError() {
			return
		}
	}
}

// write applies the plan and reflects the resulting live document into state.
// Create and Update are identical for a singleton: there is nothing to create,
// only fields to set.
func (r *settingsResource) write(ctx context.Context, plan, cfg attrGetter, state attrSetter, diags *diag.Diagnostics) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(ctx, plan))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return
	}
	siteID := site.ID
	// `site` is Computed, so record the *canonical* name the request resolved
	// to. Storing the requested name would leave it empty whenever the
	// configuration relies on the provider default, which then disagrees with
	// the name an import records for the very same site.
	diags.Append(state.SetAttribute(ctx, path.Root("site"), types.StringValue(site.Name))...)
	if diags.HasError() {
		return
	}
	fields := r.fieldsFrom(ctx, plan, cfg, diags)
	if diags.HasError() {
		return
	}
	if err := r.data.client.UpdateSetting(ctx, siteID, r.spec.doc, fields); err != nil {
		diags.AddError("Unable to update "+r.spec.typeName, err.Error())
		return
	}
	doc, err := r.data.client.GetSetting(ctx, siteID, r.spec.doc)
	if err != nil {
		diags.AddError("Unable to read "+r.spec.typeName, err.Error())
		return
	}
	diags.Append(state.SetAttribute(ctx, path.Root("id"), types.StringValue(siteID))...)
	if diags.HasError() {
		return
	}
	r.refresh(ctx, doc, state, diags)
}

func (r *settingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.State.Raw = req.Plan.Raw
	r.write(ctx, req.Plan, req.Config, &resp.State, &resp.Diagnostics)
}

func (r *settingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.State.Raw = req.Plan.Raw
	r.write(ctx, req.Plan, req.Config, &resp.State, &resp.Diagnostics)
}

func (r *settingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	resp.State.Raw = req.State.Raw

	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := id.ValueString()
	if siteID == "" {
		var err error
		if siteID, err = r.data.client.ResolveSiteID(ctx, r.siteName(ctx, req.State)); err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
	}
	doc, err := r.data.client.GetSetting(ctx, siteID, r.spec.doc)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read "+r.spec.typeName, err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(siteID))...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, doc, &resp.State, &resp.Diagnostics)
}

// Delete is a no-op: these settings always exist and are deliberately not
// reset to defaults when the resource leaves the configuration.
func (r *settingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name, e.g.
//
//	terraform import omada_alg.this Home
func (r *settingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			fmt.Sprintf("expected the site name, e.g. `terraform import omada_%s.this Home`", r.spec.typeName))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
