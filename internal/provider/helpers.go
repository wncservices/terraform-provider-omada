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

// nilToEmpty returns an empty slice for nil, so JSON marshals [] not null.
func nilToEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// intListValue converts a Go int slice to a known (never null) types.List.
func intListValue(ctx context.Context, s []int) (types.List, diag.Diagnostics) {
	vals := make([]int64, len(s))
	for i, v := range s {
		vals[i] = int64(v)
	}
	return types.ListValueFrom(ctx, types.Int64Type, vals)
}

// intSlice reads a types.List of ints into a Go slice. Null/unknown -> nil.
func intSlice(ctx context.Context, l types.List) ([]int, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var vals []int64
	d := l.ElementsAs(ctx, &vals, false)
	out := make([]int, len(vals))
	for i, v := range vals {
		out[i] = int(v)
	}
	return out, d
}
