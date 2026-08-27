// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// macType is a string type whose values compare by MAC address rather than by
// text, so `dc:a6:32:81:56:08` and `DC-A6-32-81-56-08` are the same value.
//
// This exists because the alternatives are all worse. The controller stores
// MACs upper-case and dash-separated, but Terraform will not let a provider
// rewrite a Required attribute — neither in state (an "inconsistent result
// after apply") nor during planning ("planned value does not match config
// value"). And simply keeping both spellings apart is dangerous rather than
// merely untidy: `mac` forces replacement, so an imported reservation whose
// config happened to use colons would plan a **destroy and recreate** over
// pure formatting. Semantic equality is the mechanism the framework provides
// for exactly this, and it makes the comparison correct everywhere at once.
type macType struct {
	basetypes.StringType
}

var (
	_ basetypes.StringTypable                    = macType{}
	_ basetypes.StringValuableWithSemanticEquals = macValue{}
	_ validator.String                           = validMACValidator{}
)

func (t macType) String() string { return "macType" }

func (t macType) Equal(o attr.Type) bool {
	other, ok := o.(macType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t macType) ValueType(context.Context) attr.Value { return macValue{} }

func (t macType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return macValue{StringValue: in}, nil
}

func (t macType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	sv, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T for macType", attrValue)
	}
	return macValue{StringValue: sv}, nil
}

// macValue is a single MAC address.
type macValue struct {
	basetypes.StringValue
}

func newMACValue(s string) macValue {
	return macValue{StringValue: basetypes.NewStringValue(s)}
}

func (v macValue) Type(context.Context) attr.Type { return macType{} }

func (v macValue) Equal(o attr.Value) bool {
	other, ok := o.(macValue)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals treats two spellings of the same MAC as equal, which is
// what stops a reformatted address from planning a replacement.
func (v macValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	other, ok := newValuable.(macValue)
	if !ok {
		diags.AddError("Unexpected value type", fmt.Sprintf("expected macValue, got %T", newValuable))
		return false, diags
	}
	if v.IsNull() || v.IsUnknown() || other.IsNull() || other.IsUnknown() {
		return v.StringValue.Equal(other.StringValue), diags
	}
	return omada.NormalizeMAC(v.ValueString()) == omada.NormalizeMAC(other.ValueString()), diags
}

type validMACValidator struct{}

func (validMACValidator) Description(context.Context) string {
	return "value must be a complete six-octet MAC address"
}

func (validMACValidator) MarkdownDescription(context.Context) string {
	return "value must be a complete six-octet MAC address"
}

func (validMACValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !omada.ValidMAC(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid MAC address",
			fmt.Sprintf("%q is not a complete six-octet MAC address", req.ConfigValue.ValueString()),
		)
	}
}
