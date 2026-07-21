// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// pathRoot is a small alias for path.Root to keep diagnostics call sites terse.
func pathRoot(attr string) path.Path {
	return path.Root(attr)
}

// stringListValue converts a Go string slice to a known (never null) types.List.
func stringListValue(ctx context.Context, s []string) (types.List, diag.Diagnostics) {
	if s == nil {
		s = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, s)
}

// stringSlice reads a types.List of strings into a Go slice. Null/unknown -> nil.
func stringSlice(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var out []string
	d := l.ElementsAs(ctx, &out, false)
	return out, d
}
