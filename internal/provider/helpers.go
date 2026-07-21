// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import "github.com/hashicorp/terraform-plugin-framework/path"

// pathRoot is a small alias for path.Root to keep diagnostics call sites terse.
func pathRoot(attr string) path.Path {
	return path.Root(attr)
}
