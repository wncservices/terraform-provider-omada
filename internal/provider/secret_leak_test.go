// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// checkSecretsAbsentFromState fails if any of the given secrets appears
// anywhere in Terraform state.
//
// This is the assertion that caught the real bug: marking an attribute
// Sensitive does NOT keep it out of state, because Terraform persists
// configured values regardless of what the provider reads back. Only
// WriteOnly does. Every secret-bearing resource carries this check so the
// distinction cannot quietly regress.
func checkSecretsAbsentFromState(t *testing.T, secrets ...string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		buf, err := json.Marshal(s)
		if err != nil {
			return err
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(string(buf), secret) {
				return fmt.Errorf("secret %q leaked into Terraform state", secret)
			}
		}
		return nil
	}
}
