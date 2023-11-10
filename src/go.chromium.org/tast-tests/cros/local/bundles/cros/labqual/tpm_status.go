// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package labqual

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: TPMStatus,
		Desc: "Checks that TPM is in good status",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
	})
}

// TPMStatus reads the TPM status if TPM module is present
func TPMStatus(ctx context.Context, s *testing.State) {
	out, err := testexec.CommandContext(ctx, "tpm_manager_client", "status").Output()
	if err != nil {
		s.Fatalf("TPM status command FAILED: %s", err)
	}

	status := string(out)
	if !strings.Contains(status, "status: STATUS_SUCCESS") {
		s.Fatal("TPM status request FAILED")
	}
	if !strings.Contains(status, "enabled: true") {
		s.Log("TPM is not enabled")
	}
}
