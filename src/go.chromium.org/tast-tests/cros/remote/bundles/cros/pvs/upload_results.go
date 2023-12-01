// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UploadResults,
		Desc:         "Verifies PVS test results can be uploaded successfully",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// UploadResults runs pvs run then upload to mimmic the results upload flow.
func UploadResults(ctx context.Context, s *testing.State) {
	pvsRunner := pvsutils.PVSRunner{
		Dut:         s.DUT().Conn(),
		ContainerID: s.FixtValue().(string),
	}
	pvsRunner.RunPVSCommand(ctx, s, "run --test tast.example.Pass")
	pvsRunner.RunPVSCommandNonfatal(ctx, "upload")
}
