// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Enrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enroll a device without checking policies",
		BugComponent: "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
		},
		Attr:         []string{"group:enrollment"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
		},
		Fixture: fixture.CleanOwnership,
		Timeout: 4 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Enroll an unmanaged device by entering credentials manually to
			// ensure it enrolls with user interaction.
			// COM_FOUND_CUJ26_TASK3_WF1
			Value: "screenplay-e3feb0c8-a73b-4974-acf6-310348498e62",
		}},
	})
}

func Enrollment(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	pJSON, err := json.Marshal(policy.NewBlob())
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChromeAndFakeDMS(cleanupCtx, &empty.Empty{})
}
