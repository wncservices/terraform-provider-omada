// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import "testing"

func TestValidMAC(t *testing.T) {
	t.Parallel()
	for _, mac := range []string{
		"00-11-22-33-44-55",
		"00:11:22:33:44:55",
		"00.11.22.33.44.55",
		"aA:bB:cC:dD:eE:fF",
	} {
		if !ValidMAC(mac) {
			t.Errorf("ValidMAC(%q) = false", mac)
		}
	}
	for _, mac := range []string{
		"",
		"00-11-22-33-44",
		"00-11-22-33-44-55-66",
		"0011.2233.4455",
		"../clients",
		"00-11-22-33-44-5Z",
	} {
		if ValidMAC(mac) {
			t.Errorf("ValidMAC(%q) = true", mac)
		}
	}
}
