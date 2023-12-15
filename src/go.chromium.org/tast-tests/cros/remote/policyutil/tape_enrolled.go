// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	tapeFixturetotalRunTime = 30 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.TAPEEnrolled,
		Desc: "Leases an account using TAPE and enrolls using real DMServer",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		Impl:            &tapeEnrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.baserpc.FileSystem",
			"tast.cros.tape.Service",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

type tapeEnrolledFixt struct {
	accountManager *tape.OwnedTestAccountManager
	account        *tape.OwnedTestAccount
	rpcClient      *rpc.Client
}

func (e *tapeEnrolledFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ok := false

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	// Make sure to clean up on error.
	defer func(ctx context.Context) {
		if !ok {
			if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
				s.Error("Failed to reset TPM after setup failure: ", err)
			}
		}
	}(cleanupCtx)

	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer rpcClient.Close(cleanupCtx)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(tapeFixturetotalRunTime.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer func(ctx context.Context) {
		if !ok {
			accountManager.CleanUp(cleanupCtx)
		}
	}(cleanupCtx)

	defer func(ctx context.Context) {
		if !ok {
			if err := tapeClient.DeprovisionHelper(ctx, rpcClient, account.CustomerID, account.OrgUnitPath); err != nil {
				s.Fatal("Failed to deprovision device: ", err)
			}
		}
	}(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username: account.Username,
		Password: account.Password,
	}); err != nil {
		s.Fatal("Failed to enroll using Chrome: ", err)
	}

	if _, err := policyClient.StopChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop Chrome: ", err)
	}

	e.rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Error("Failed to connect to DUT: ", err)
	}

	e.accountManager = accountManager
	e.account = account

	ok = true

	return &fixture.TAPEEnrolledFixtData{
		Username:  account.Username,
		Password:  account.Password,
		RequestID: account.RequestID,
	}
}

func (e *tapeEnrolledFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	defer func(ctx context.Context) {
		if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to reset TPM: ", err)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	if e.rpcClient != nil {
		e.rpcClient.Close(ctx)
	}

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	if e.account != nil {
		tapeClient.ReleaseOwnedTestAccount(ctx, e.account)
	}

	if e.accountManager != nil {
		if err := tapeClient.DeprovisionHelper(ctx, cl, e.account.CustomerID, e.account.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}

		e.accountManager.CleanUp(ctx)
	}
}

// Check if device state has been lost.
func (e *tapeEnrolledFixt) Reset(ctx context.Context) error {
	return checkEnrollment(ctx, e.rpcClient)
}

func (*tapeEnrolledFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {}

// Tests need to make sure not to clear enrollment. Report a test error if it happens.
func (e *tapeEnrolledFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := checkEnrollment(ctx, e.rpcClient); err != nil {
		s.Error("Enrollement lost: ", err)
	}
}
