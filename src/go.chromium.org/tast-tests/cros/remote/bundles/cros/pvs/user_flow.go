// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UserFlow,
		Desc:         "Verifies the happy path PVS fetch, run, list, upload user flow works",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"bbrotherton@google.com",
		},
		Attr:    []string{"group:pvs"},
		Timeout: 600 * time.Minute,
		Fixture: "pvsShopUnpack",
		Vars: []string{
			"pvs.UserFlow.requirementFilterOverride",
			"pvs.UserFlow.ignorePVSRunFailure",
		},
	})
}

// UserFlow runs pvs fetch, list, run, and upload to mimmic the partner user flow. If no
// pvs.UserFlow.requirementFilterOverride is provided, "boot-perf" will be used as the
// requirement filter when running through the user flow.
func UserFlow(ctx context.Context, s *testing.State) {
	pvsRunner := pvsutils.PVSRunner{
		Dut:         s.DUT().Conn(),
		ContainerID: s.FixtValue().(string),
	}
	requirementFilter := "boot-perf"
	if requirementFilterOverride, ok := s.Var("pvs.UserFlow.requirementFilterOverride"); ok {
		requirementFilter = requirementFilterOverride
	}
	s.Log("PVS is being run with the following filter: ", requirementFilter)
	if _, _, err := pvsRunner.RunPVSCommandNonfatal(ctx, "fetch"); err != nil {
		s.Error("Error occured when calling `pvs fetch`: ", err)
	}
	runSubcommand := fmt.Sprintf("run --filter %v", requirementFilter)
	if _, _, err := pvsRunner.RunPVSCommandNonfatal(ctx, runSubcommand); err != nil {
		if _, ok := s.Var("pvs.UserFlow.ignorePVSRunFailure"); !ok {
			s.Error("Error occured when calling `pvs run`: ", err)
		} else {
			s.Log("Error occured when calling `pvs run`: ", err)
		}
	}
	listSubcommand := fmt.Sprintf("list --filter %v", requirementFilter)
	if _, _, err := pvsRunner.RunPVSCommandNonfatal(ctx, listSubcommand); err != nil {
		s.Error("Error occured when calling `pvs list`: ", err)
	}
	if _, _, err := pvsRunner.RunPVSCommandNonfatal(ctx, "upload"); err != nil {
		s.Error("Error occured when calling `pvs upload`: ", err)
	}
}
