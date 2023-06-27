// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Fail,
		Desc: "Always fails to verify hwsec nightly & weekly test scheduling",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com", // Test author.
		},
		// ChromeOS > Platform > System > Hardware Security > HwSec AP
		BugComponent: "b:1188704",
		Attr:         []string{"group:hwsec", "hwsec_nightly", "hwsec_weekly"},
		Params: []testing.Param{{
			Name:              "hw_tpm1",
			ExtraSoftwareDeps: []string{"no_tpm2_simulator", "no_tpm_dynamic", "tpm1"},
		}, {
			Name:              "hw_gsc",
			ExtraSoftwareDeps: []string{"no_tpm2_simulator", "no_tpm_dynamic", "gsc"},
		}, {
			Name:              "hw_tpm_dynamic",
			ExtraSoftwareDeps: []string{"no_tpm2_simulator", "tpm_dynamic"},
		}, {
			Name:              "vm_tpm2",
			ExtraSoftwareDeps: []string{"tpm2_simulator", "no_tpm_dynamic", "no_gsc"},
		}, {
			Name:              "vm_ti50",
			ExtraSoftwareDeps: []string{"tpm2_simulator", "no_tpm_dynamic", "gsc"},
		}, {
			Name:              "vm_tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm2_simulator", "tpm_dynamic"},
		}},
	})
}

func Fail(ctx context.Context, s *testing.State) {
	s.Log("Here's an informative message")
	s.Error("Here's an error")
	s.Error("And here's a second")
	s.Fatal("Finally, a fatal error")
}
