// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/arcent"
	"chromiumos/tast/local/chrome/familylink"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornBlockedApps,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if blocked apps cannot be installed from Child Account",
		Contacts:     []string{"mhasank@google.com", "arc-commercial@google.com"},
		Attr:         []string{"group:mainline", "informational", "group:arc-functional"},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"play_store",
		},
		Timeout: 6 * time.Minute,
		VarDeps: []string{"arc.parentUser", "arc.parentPassword", "arc.childUser", "arc.childPassword"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Fixture: "familyLinkUnicornArcPolicyLogin",
	})
}

func UnicornBlockedApps(ctx context.Context, s *testing.State) {
	const (
		provisioningTimeout = 3 * time.Minute
		blockedPackage      = "com.google.android.apps.youtube.creator"
	)
	fdms := s.FixtValue().(*familylink.FixtData).FakeDMS
	cr := s.FixtValue().(*familylink.FixtData).Chrome
	tconn := s.FixtValue().(*familylink.FixtData).TestConn
	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	blockedApps := []policy.Application{
		{
			PackageName: blockedPackage,
			InstallType: "BLOCKED",
		},
	}
	blockedAppsPolicy := &policy.ArcPolicy{
		Val: &policy.ArcPolicyValue{
			Applications: blockedApps,
		},
	}
	policies := []policy.Policy{blockedAppsPolicy, arcEnabledPolicy}
	pb := policy.NewBlob()
	pb.PolicyUser = s.FixtValue().(*familylink.FixtData).PolicyUser
	pb.AddPolicies(policies)

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to connect to ARC: ", err)
	}
	defer a.Close(ctx)

	verboseTags := []string{"clouddpc", "Finsky", "Volley", "PlayCommon"}
	if err := a.EnableVerboseLogging(ctx, verboseTags...); err != nil {
		s.Fatal("Unable to change log level: ", err)
	}

	if err := arcent.ConfigureProvisioningLogs(ctx, a); err != nil {
		s.Fatal("Unable to configure provisioning logs: ", err)
	}

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(ctx)

	// Blocked app should either not install or immediately uninstall after installation.
	if err := arcent.ValidateBlockedAppInstall(ctx, tconn, a, d, blockedPackage, 5*time.Minute); err != nil {
		s.Fatal("Failed to verify blocked app uninstall: ", err)
	}

}
