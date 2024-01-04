// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type unaffiliatedDevicesArcRestrictionTestArgs struct {
	arcEnabledPolicy                   bool
	unaffiliatedDeviceArcAllowedPolicy bool
	shouldEnroll                       bool
	isAffiliated                       bool
	expectArcEnabled                   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnaffiliatedDevicesArcRestriction,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if ARC is booted when policy is true and not when policy is false",
		Contacts:     []string{"arc-commercial@google.com", "preranap@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      8 * time.Minute,
		Fixture:      fixture.CleanOwnership,
		VarDeps: []string{
			arcent.LoginPoolVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.UnaffiliatedDeviceArcAllowed{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_true_on_unaffiliated_vm",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_true_on_unaffiliated",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_true_on_affiliated_vm",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       true,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_true_on_affiliated",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       true,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_false_on_affiliated_vm",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: false,
					isAffiliated:                       true,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "arc_allowed_when_unaffiliated_device_arc_allowed_is_false_on_affiliated",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: false,
					isAffiliated:                       true,
					shouldEnroll:                       true,
					expectArcEnabled:                   true,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "arc_disallowed_when_unaffiliated_device_arc_allowed_is_false_on_unaffiliated_vm",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: false,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   false,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "arc_disallowed_when_unaffiliated_device_arc_allowed_is_false_on_unaffiliated",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   true,
					unaffiliatedDeviceArcAllowedPolicy: false,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   false,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name: "arc_disabled_when_arc_enabled_is_false_and_unaffiliated_device_arc_allowed_is_true_vm",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   false,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   false,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			},
			{
				Name: "arc_disabled_when_arc_enabled_is_false_and_unaffiliated_device_arc_allowed_is_true",
				Val: unaffiliatedDevicesArcRestrictionTestArgs{
					arcEnabledPolicy:                   false,
					unaffiliatedDeviceArcAllowedPolicy: true,
					isAffiliated:                       false,
					shouldEnroll:                       true,
					expectArcEnabled:                   false,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			}},
	})
}

func UnaffiliatedDevicesArcRestriction(ctx context.Context, s *testing.State) {
	args := s.Param().(unaffiliatedDevicesArcRestrictionTestArgs)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()
	creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
	if err != nil {
		s.Fatal("Get login creds: ", err)
	}

	fdms, err := fakedms.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to create fakedms: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	policies := []policy.Policy{&policy.ArcEnabled{Val: args.arcEnabledPolicy},
		&policy.UnaffiliatedDeviceArcAllowed{Val: args.unaffiliatedDeviceArcAllowedPolicy}}

	cr, err := loginAndConnectToChrome(ctx, cleanupCtx, fdms, args.shouldEnroll, args.isAffiliated, policies, creds)
	if err != nil {
		s.Fatal("Connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err == nil {
		defer a.Close(ctx)
		if !args.expectArcEnabled {
			s.Fatal("Started ARC while blocked by user policy")
		}
	} else if args.expectArcEnabled {
		s.Fatal("Start ARC by user policy: ", err)

	}

}

func loginAndConnectToChrome(ctx, cleanupCtx context.Context, fdms *fakedms.FakeDMS, shouldEnroll,
	isAffiliated bool, policies []policy.Policy, creds credconfig.Creds) (*chrome.Chrome, error) {
	var opts []chrome.Option

	blob := policy.NewBlob()
	blob.DeviceAffiliationIds = []string{"affiliation_id"}
	blob.PolicyUser = creds.User

	//UserAffiliatedIds is same as DeviceAffiliationIds if the user is affiliated
	if isAffiliated {
		blob.UserAffiliationIds = []string{"affiliation_id"}
	}

	if err := blob.AddPolicies(policies); err != nil {
		return nil, errors.Wrap(err, "failed to add policy to policy blob")
	}

	if err := fdms.WritePolicyBlob(blob); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob to fdms")
	}

	if shouldEnroll {
		opts = append(opts, chrome.FakeEnterpriseEnroll(creds))
	}
	opts = append(opts, chrome.GAIALogin(creds))
	opts = append(opts, chrome.ARCSupported())
	opts = append(opts, chrome.UnRestrictARCCPU())
	opts = append(opts, chrome.DMSPolicy(fdms.URL))
	opts = append(opts, chrome.EnableFeatures("UnaffiliatedDeviceArcRestriction"))
	opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
	cr, err := chrome.New(
		ctx,
		opts...)

	return cr, err
}
