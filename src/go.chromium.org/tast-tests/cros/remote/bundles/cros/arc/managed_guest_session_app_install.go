// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/arc"
	"go.chromium.org/tast/core/rpc"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const mgsTestTimeout = 15 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:     ManagedGuestSessionAppInstall,
		Desc:     "Verifies app is force-installed in MGS",
		Contacts: []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:dpanel-end2end", "group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome", "play_store"},
		Timeout:      mgsTestTimeout,
		Fixture:      fixture.CleanOwnership,
		VarDeps: []string{
			tape.ServiceAccount1,
			tape.ServiceAccount2,
		},
		ServiceDeps: []string{
			"tast.cros.arc.MGSService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.tape.Service",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"group:hw_agnostic"},
			}},
	})
}

// ManagedGuestSessionAppInstall verifies that an app is force installed in MGS.
func ManagedGuestSessionAppInstall(ctx context.Context, s *testing.State) {
	const (
		testPackage = "com.google.android.keep"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	tapeClient, err := tape.GetClientFromLocalCredentials(ctx, []string{s.RequiredVar(tape.ServiceAccount1), s.RequiredVar(tape.ServiceAccount2)})
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, false, tape.WithTimeout(int32(mgsTestTimeout.Seconds())), tape.WithPoolID(tape.ArcSnapshot))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	// Deprovision the DUT at the end of the test. As devices might get
	// provisioned even when the enrollment fails we need to defer the
	// deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(ctx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	service := arc.NewMGSServiceClient(cl.Conn)

	s.Log("Enrolling device")
	if _, err = service.Enroll(ctx, &arc.EnrollRequest{User: acc.Username, Pass: acc.Password}); err != nil {
		s.Fatal("Remote call Enroll() failed: ", err)
	}

	s.Log("Restarting session for MGS to start automatically")
	if _, err = service.RestartSession(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Remote call RestartSession() failed: ", err)
	}

	if _, err := service.WaitForPackages(ctx, &arc.WaitForPackagesRequest{Packages: []string{testPackage}}); err != nil {
		s.Fatal("Remote call WaitForPackages() failed: ", err)
	}
}
