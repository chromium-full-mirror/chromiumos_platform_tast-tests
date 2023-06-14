// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RecoveryCrypto,
		Desc: "Checks that cryptohome recovery process succeeds with fake/local mediation",
		Contacts: []string{
			"cryptohome-core@google.com",
			"cros-lurs@google.com",
			"anastasiian@chromium.org",
		},
		BugComponent: "b:1148604", // ChromeOS > Security > Cryptohome > Cryptohome Recovery
		Attr:         []string{"group:mainline", "group:cryptohome"},
		SoftwareDeps: []string{"tpm"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"no_tpm_dynamic"},
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
		}},
	})
}

func RecoveryCrypto(ctx context.Context, s *testing.State) {
	cmdRunner := hwseclocal.NewCmdRunner()
	testTool, newErr := cryptohomecommon.NewRecoveryTestToolWithFakeMediator(cmdRunner)
	if newErr != nil {
		s.Fatal("Failed to initialize RecoveryTestTool", newErr)
	}
	defer func(s *testing.State, testTool *cryptohomecommon.RecoveryTestTool) {
		if err := testTool.RemoveDir(); err != nil {
			s.Error("Failed to remove dir: ", err)
		}
	}(s, testTool)

	if err := testTool.CreateHsmPayload(ctx); err != nil {
		s.Fatal("Failed to execute CreateHsmPayload: ", err)
	}

	if err := testTool.CreateRecoveryRequest(ctx); err != nil {
		s.Fatal("Failed to execute CreateRecoveryRequest: ", err)
	}

	if err := testTool.FakeMediate(ctx); err != nil {
		s.Fatal("Failed to execute FakeMediate: ", err)
	}

	if err := testTool.Decrypt(ctx); err != nil {
		s.Fatal("Failed to execute Decrypt: ", err)
	}

	if err := testTool.Validate(ctx); err != nil {
		s.Fatal("Failed to validate: ", err)
	}
}
