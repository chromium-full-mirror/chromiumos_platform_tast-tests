// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast/core/testing"
)

const (
	tapeFixtureTotalRunTime = 30 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.TAPEAccount,
		Desc: "Leases an account using TAPE",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		Impl:            &tapeAccountFixt{},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
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

type tapeAccountFixt struct {
	accountManager *tape.OwnedTestAccountManager
	account        *tape.OwnedTestAccount
}

func (e *tapeAccountFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(tapeFixtureTotalRunTime.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}

	e.accountManager = accountManager
	e.account = account

	return &fixture.TAPEAccountData{
		Username:    account.Username,
		Password:    account.Password,
		RequestID:   account.RequestID,
		CustomerID:  account.CustomerID,
		OrgUnitPath: account.OrgUnitPath,
	}
}

func (e *tapeAccountFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := e.accountManager.CleanUp(ctx); err != nil {
		s.Error("Failed to clean up accountManager: ", err)
	}
}

func (e *tapeAccountFixt) Reset(ctx context.Context) error {
	// TODO(b/316326530): Reset tape account

	return nil
}

func (*tapeAccountFixt) PreTest(ctx context.Context, s *testing.FixtTestState)    {}
func (e *tapeAccountFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}
