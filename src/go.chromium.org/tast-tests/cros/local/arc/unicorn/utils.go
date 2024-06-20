// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unicorn contains utility methods for testing unicorn account in ARC.
package unicorn

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
)

// SetUpFakePolicyServer sets up a fake policy server for unicorn account.
func SetUpFakePolicyServer(ctx context.Context, outDir, policyUser string, policies []policy.Policy) (fdms *fakedms.FakeDMS, retErr error) {
	fdms, err := fakedms.New(ctx, outDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create fakedms")
	}
	defer func() {
		if retErr != nil {
			fdms.Stop(ctx)
		}
	}()

	pb := policy.NewBlob()
	pb.UseUniversalSigningKeys = true
	pb.PolicyUser = policyUser
	if err := pb.AddPolicies(policies); err != nil {
		return nil, errors.Wrap(err, "failed to add policy to policy blob")
	}
	if err := fdms.WritePolicyBlob(pb); err != nil {
		return nil, errors.Wrap(err, "failed to write policy blob to fdms")
	}

	return fdms, nil
}

// StartChromeWithARC starts Chrome with child account and ARC enabled.
func StartChromeWithARC(ctx context.Context, childUser, childPassword, parentUser, parentPassword, fdmsURL string) (*chrome.Chrome, error) {
	return chrome.New(ctx,
		chrome.GAIALogin(chrome.Creds{
			User:       childUser,
			Pass:       childPassword,
			ParentUser: parentUser,
			ParentPass: parentPassword,
		}),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdmsURL),
		// Family Link users look like consumer users with
		// @gmail.com emails but require policy. Since policy
		// key verification doesn't work for gmail users,
		// disable it.
		chrome.DisablePolicyKeyVerification(),
		chrome.ExtraArgs(append(arc.DisableSyncFlags(), "--force-devtools-available")...))
}
