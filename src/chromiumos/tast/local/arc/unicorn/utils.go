// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package unicorn contains utility methods for testing unicorn account in ARC.
package unicorn

import (
	"context"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/chrome"
	"go.chromium.org/tast/core/errors"
)

const (
	// ParentUserVar is the username of the parent
	ParentUserVar = "arc.parentUser"
	// ParentPasswordVar is the password of the parent
	ParentPasswordVar = "arc.parentPassword"
	// ChildUserVar is the username of the child
	ChildUserVar = "arc.childUser"
	// ChildPasswordVar is the password of the child
	ChildPasswordVar = "arc.childPassword"
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
