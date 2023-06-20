// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ClearOwnership,
		Desc:         "Verifies that the TPM ownership can be cleared",
		Contacts:     []string{"cros-hwsec@google.com", "yich@google.com"},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"reboot", "tpm_clear_allowed"},
		Attr:         []string{"group:hwsec_destructive_func", "group:tpm_manager"},
		ServiceDeps:  []string{"tast.cros.hwsec.AttestationDBusService"},
	})
}

func ClearOwnership(ctx context.Context, s *testing.State) {
	r := hwsecremote.NewCmdRunner(s.DUT())

	helper, err := hwsecremote.NewFullHelper(r, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}

	s.Log("Start resetting TPM if needed")
	if err := helper.EnsureTPMIsReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}
	s.Log("TPM is confirmed to be reset")
}
