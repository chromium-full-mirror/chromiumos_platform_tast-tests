// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/version"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/lacros/provision"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/lacros/update"
	lacrosservice "go.chromium.org/tast-tests/cros/services/cros/lacros"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type testCaseRtoS struct {
	skew      *version.Delta
	validSkew bool // true if it's a valid supported skew
}

var (
	// Version to increment from rootfs.
	statefulIsNewerThanRootfs = version.NewDelta(1, 0, 0, 0)
	statefulIsSameAsRootfs    = version.NewDelta(0, 0, 0, 0)
	statefulIsTooNew          = version.NewDelta(3, 0, 0, 0)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UpdateRootfsToStateful,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that Stateful Lacros is selected when it is newer than Rootfs Lacros",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "lacros-team@google.com", "hyungtaekim@chromium.org"},
		BugComponent: "crbug:OS>LaCrOS",
		// TODO(b/287277678): Test is disabled in continuous testing until root caused. Change Attr to string{"group:mainline", "informational"},
		Attr:         []string{},
		SoftwareDeps: []string{"chrome", "lacros"},
		ServiceDeps:  []string{"tast.cros.lacros.UpdateTestService"},
		// lacrosComponent is a runtime var to specify a name of the component which Lacros is provisioned to.
		Vars: []string{"lacrosComponent"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Val:               testCaseRtoS{skew: statefulIsNewerThanRootfs, validSkew: true},
		}, {
			Name:              "unstable",
			ExtraSoftwareDeps: []string{"lacros_unstable"},
			Val:               testCaseRtoS{skew: statefulIsNewerThanRootfs, validSkew: true},
		}, {
			Name:              "no_skew",
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Val: testCaseRtoS{
				skew:      statefulIsSameAsRootfs, // no skew. rootfs-lacros and stateful-lacros will be the same version. stateful-lacros should be used.
				validSkew: true,
			},
		}, {
			Name:              "invalid_skew",
			ExtraSoftwareDeps: []string{"lacros_stable"},
			Val: testCaseRtoS{
				skew:      statefulIsTooNew, // invalid skew; + 3 milestone newer than ash-chrome is not supported (crbug.com/1258138)
				validSkew: false,
			},
		}},
		Timeout: 5 * time.Minute,
	})
}

func UpdateRootfsToStateful(ctx context.Context, s *testing.State) {
	// Create a UpdateTestService client.
	conn, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}
	defer conn.Close(ctx)
	utsClient := lacrosservice.NewUpdateTestServiceClient(conn.Conn)

	// Bump up the major version of Stateful Lacros to be newer than of Rootfs
	// one in order to simulate the desired test scenario (Rootfs => Stateful).
	rootfsLacrosVersion, err := update.GetRootfsLacrosVersion(ctx, s.DUT(), utsClient)
	if err != nil {
		s.Fatal("Failed to get the Rootfs Lacros version: ", err)
	}
	ashVersion, err := update.GetAshVersion(ctx, s.DUT(), utsClient)
	if err != nil {
		s.Fatal("Failed to get the Ash version: ", err)
	}

	skew := s.Param().(testCaseRtoS).skew
	validSkew := s.Param().(testCaseRtoS).validSkew
	statefulLacrosVersion := rootfsLacrosVersion.Increment(skew)
	if !statefulLacrosVersion.IsValid() {
		s.Fatal("Invalid Stateful Lacros version: ", statefulLacrosVersion)
	} else if rootfsLacrosVersion.IsNewerThan(statefulLacrosVersion) {
		s.Fatalf("Invalid Stateful Lacros version: %v, should not be older than Rootfs: %v", statefulLacrosVersion, rootfsLacrosVersion)
	} else if err := version.IsSkewValid(statefulLacrosVersion, ashVersion); (err == nil) != validSkew {
		s.Fatalf("Should be a valid skew? expected: %v, actual: %v, %v", validSkew, (err == nil), err)
	}

	// Get the component to override from the runtime var. Defaults to Lacros dev channel.
	overrideComponent, err := update.LacrosComponentVar(s)
	if err != nil {
		s.Fatal("Failed to get Lacros component: ", err)
	}

	// Deferred cleanup to always reset to the previous state with no provisioned files.
	ctxForCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		if err := update.ClearLacrosUpdate(ctx, utsClient); err != nil {
			s.Log("Failed to clean up provisioned Lacros: ", err)
		}
	}(ctxForCleanup)

	// Provision Stateful Lacros from the Rootfs Lacros image file with the simulated version and component.
	if err := update.ProvisionLacrosFromRootfsLacrosImagePath(ctx, provision.TLSAddrVar.Value(), s.DUT(), statefulLacrosVersion.String(), overrideComponent); err != nil {
		s.Fatal("Failed to provision Stateful Lacros from Rootfs image source: ", err)
	}

	// Verify that the expected Stateful Lacros version/component is selected.
	if err := update.VerifyLacrosUpdate(ctx, lacrosservice.BrowserType_LACROS_STATEFUL, statefulLacrosVersion.String(), overrideComponent, utsClient); err != nil {
		s.Fatal("Failed to verify provisioned Lacros version: ", err)
	}
}
