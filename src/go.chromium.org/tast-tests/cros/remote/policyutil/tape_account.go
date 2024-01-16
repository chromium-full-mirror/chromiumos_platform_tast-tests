// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	tapeFixtureTotalRunTime = 30 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.TAPEAccount,
		Desc: "Leases an account from the default managed pool using TAPE",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.DefaultManaged,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectDisabledAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Ash web protect disabled using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSAshWebProtectDisabled,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledAllowAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Ash web protect enabled and allowed using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSAshWebProtectEnabledAllow,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledBlockAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Ash web protect enabled and blocked using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSAshWebProtectEnabledBlock,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSLacrosWebProtectDisabledAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Lacros web protect disabled using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSLacrosWebProtectDisabled,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSLacrosWebProtectEnabledAllowAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Lacros web protect enabled and allowed using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSLacrosWebProtectEnabledAllow,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSLacrosWebProtectEnabledBlockAccount,
		Desc: "Leases an account with Autolaunch MGS and pre-applied policies for Lacros web protect enabled and blocked using TAPE",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		Impl: &tapeAccountFixt{
			poolID:                tape.EnterpriseConnectorsMGSLacrosWebProtectEnabledBlock,
			accountLeasingTimeout: tapeFixtureTotalRunTime,
		},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    30 * time.Second,
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
	accountManager    *tape.OwnedTestAccountManager
	account           *tape.OwnedTestAccount
	serviceAccountVar []byte
	poolID            string
	// accountLeasingTimeout is the total timeout for leasing a tape account for all the tests using the fixture.
	// If the number of tests using the fixture increases, then this duration should increase as well.
	accountLeasingTimeout time.Duration
}

func (e *tapeAccountFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	e.serviceAccountVar = []byte(s.RequiredVar(tape.ServiceAccountVar))
	tapeClient, err := tape.NewClient(ctx, e.serviceAccountVar)
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(e.accountLeasingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of all the tests using this fixture.
	accountManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(e.poolID))
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

	// Clean up account between tests.
	tapeClient, err := tape.NewClient(ctx, e.serviceAccountVar)
	if err != nil {
		return errors.Wrap(err, "failed to create tape client")
	}
	if err = tapeClient.CleanUpAccount(ctx, e.account.RequestID); err != nil {
		return errors.Wrap(err, "failed to clean up the account")
	}

	return nil
}

func (*tapeAccountFixt) PreTest(ctx context.Context, s *testing.FixtTestState)    {}
func (e *tapeAccountFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}
