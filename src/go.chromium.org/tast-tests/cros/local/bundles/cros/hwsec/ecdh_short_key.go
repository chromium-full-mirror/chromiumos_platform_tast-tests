// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"os"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECDHShortKey,
		Desc: "Verifies a short ECC key (leading byte(s) of X/Y coordinate being 0) works correctly",
		Contacts: []string{
			"cros-hwsec@google.com",
			"yich@google.com", // Test author.
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:hwsec", "hwsec_nightly", "group:hwsec_infra"},
		SoftwareDeps: []string{"tpm2"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"no_tpm_dynamic"},
		}, {
			Name:              "tpm_dynamic",
			ExtraSoftwareDeps: []string{"tpm_dynamic"},
			ExtraHardwareDeps: hwdep.D(hwdep.HasTpm2()),
		}},
	})
}

// ECDHShortKey verifies ECDD can work with short ECC keys.
func ECDHShortKey(ctx context.Context, s *testing.State) {

	const filename = "/tmp/keyblob"

	// Create a key for testing.
	if _, err := testexec.CommandContext(ctx, "trunks_client", "--key_create", "--ecc", "--usage=all", "--key_blob="+filename).Output(); err != nil {
		s.Fatal("Failed to create an ECC key: ", err)
	}

	defer func() {
		if err := os.Remove(filename); err != nil {
			s.Fatal("Failed to remove key blob file: ", err)
		}
	}()

	// Load the key back to TPM.
	out, err := testexec.CommandContext(ctx, "trunks_client", "--key_load", "--key_blob="+filename).Output()
	if err != nil {
		s.Fatal("Failed to load the key from key blob: ", err)
	}
	// Parse the handle from the command output. The example output is "Loaded key handle: 0x80000003".
	re := regexp.MustCompile(`0[xX][0-9a-fA-F]+`)
	matches := re.FindAllString(string(out), -1)
	if matches == nil || len(matches) != 1 {
		s.Fatal("Failed to parse loaded key handle: ", out)
	}

	handle := matches[0]

	defer func() {
		if _, err := testexec.CommandContext(ctx, "trunks_client", "--key_unload", "--handle="+handle).Output(); err != nil {
			s.Errorf("Failed to unload key handle 0x%x with error: %v",
				handle, err)
		}
	}()

	// Test if the TPM key can work with a short remote ECC key.
	if out, err := testexec.CommandContext(ctx, "trunks_client", "--key_test_short_ecc", "--handle="+handle).Output(); err != nil {
		s.Error("Failed to perform ECDH with a short key: ", err)
		// All the output is valuable for debugging; Hopefully it is short enough (around 4 lines).
		s.Fatal(out)
	}
}
