// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

//go:build tools

// Package tools pins developer tooling (tfplugindocs) as a build dependency so
// `go generate ./...` uses a version tracked in go.mod. It is never compiled
// into the provider binary (guarded by the `tools` build tag).
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
