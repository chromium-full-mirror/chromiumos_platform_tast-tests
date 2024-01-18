// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/version"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/lacros/provision"
	lacrosupdate "go.chromium.org/tast-tests/cros/remote/bundles/cros/lacros/update"
	lacrosservice "go.chromium.org/tast-tests/cros/services/cros/lacros"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type testCaseStoR struct {
	skew *version.Delta
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         UpdateStatefulToRootfs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that rootfs-lacros is selected when stateful-lacros is older than or equal to rootfs-lacros or stateful-lacros is incompatible with ash-chrome",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "lacros-team@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "b:1456869",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		ServiceDeps:  []string{"tast.cros.lacros.UpdateTestService"},
		// lacrosComponent is a runtime var to specify a name of the component which Stateful Lacros is provisioned to.
		Vars: []string{"lacrosComponent"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Val: testCaseStoR{
				skew: version.NewDelta(0, 0, 1000, 0), // 0 major -1000 build version skew from rootfs-lacros
			},
		}, {
			Name:              "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			Val: testCaseStoR{
				skew: version.NewDelta(0, 0, 1000, 0), // 0 major -1000 build version skew from rootfs-lacros
			},
		}, {
			Name:              "invalid_skew",
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Val: testCaseStoR{
				skew: version.NewDelta(10, 0, 0, 0), // invalid skew; -10 milestone older than ash-chrome. if stateful-lacros is incompatible with ash-chrome, rootfs-lacros should be used.
			},
		}},
		Timeout: 5 * time.Minute,
	})
}

func UpdateStatefulToRootfs(ctx context.Context, s *testing.State) {
	// Create a UpdateTestService client.
	conn, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}
	defer conn.Close(ctx)
	utsClient := lacrosservice.NewUpdateTestServiceClient(conn.Conn)

	// Set the version of Stateful Lacros.
	// In this test Stateful Lacros needs to be older than Rootfs Lacros (so, Lacros should be open from Rootfs)
	// In reality it is unlikely to happen unless it fails to install the latest Stateful Lacros from Omaha.
	rootfsLacrosVersion, err := lacrosupdate.GetRootfsLacrosVersion(ctx, s.DUT(), utsClient)
	if err != nil {
		s.Fatal("Failed to get the Rootfs Lacros version: ", err)
	}
	ashVersion, err := lacrosupdate.GetAshVersion(ctx, s.DUT(), utsClient)
	if err != nil {
		s.Fatal("Failed to get the Ash version: ", err)
	}

	skew := s.Param().(testCaseStoR).skew
	statefulLacrosVersion := rootfsLacrosVersion.Decrement(skew)
	s.Logf("Versions: ash=%s rootfs-lacros=%s stateful-lacros=%s", ashVersion.String(), rootfsLacrosVersion.String(), statefulLacrosVersion.String())
	if !statefulLacrosVersion.IsValid() {
		s.Fatal("Invalid Stateful Lacros version: ", statefulLacrosVersion)
	} else if statefulLacrosVersion.IsNewerThan(rootfsLacrosVersion) {
		s.Fatalf("Invalid Stateful Lacros version: %v, should be older than or equal to Rootfs: %v", statefulLacrosVersion, rootfsLacrosVersion)
	}

	// Get the component to override from the runtime var. Defaults to Lacros dev channel.
	statefulLacrosComponent, err := lacrosupdate.LacrosComponentVar(s)
	if err != nil {
		s.Fatal("Failed to get Lacros component: ", err)
	}

	// Deferred cleanup to always reset to the previous state with no provisioned files.
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := lacrosupdate.ClearLacrosUpdate(ctx, utsClient); err != nil {
			s.Log("Failed to clean up provisioned Lacros: ", err)
		}
	}(ctxForCleanup)

	// Simulate that an older version of Stateful Lacros has been installed than Rootfs Lacros.
	if err := lacrosupdate.ProvisionLacrosFromRootfsLacrosImagePath(ctx, provision.TLSAddrVar.Value(), s.DUT(), statefulLacrosVersion.String(), statefulLacrosComponent); err != nil {
		s.Fatal("Failed to provision Stateful Lacros from Rootfs image source: ", err)
	}

	// Verify that a newer version (Rootfs Lacros) is selected.
	if err := lacrosupdate.VerifyLacrosUpdate(ctx, lacrosservice.BrowserType_LACROS_ROOTFS, rootfsLacrosVersion.String(), "" /* no component for rootfs lacros */, utsClient); err != nil {
		s.Fatal("Failed to verify provisioned Lacros version: ", err)
	}
}
