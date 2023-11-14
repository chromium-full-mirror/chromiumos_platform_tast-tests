// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TPMReset,
		Desc: "Resets TPM on the Device",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		Params: []testing.Param{
			{
				Name:    "rec_mode",
				Val:     fixture.RecModeNoServices,
				Fixture: fixture.RecModeNoServices,
			},
		},
		VarDeps:      []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		TestBedDeps:  []string{tbdep.ServoUSBState("NORMAL"), tbdep.ServoState("WORKING")},
	})
}

// TPMReset runs TPM recovery if TPM is present.
func TPMReset(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	s.Log("Running TPM recovery command - chromeos-tpm-recovery")
	cmd := h.DUT.Conn().CommandContext(ctx, "chromeos-tpm-recovery")
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.Fatal("Failed to run chromeos-tpm-recovery : ", string(out))
	}
	s.Log("TPM recovery command output : ", out)
}
