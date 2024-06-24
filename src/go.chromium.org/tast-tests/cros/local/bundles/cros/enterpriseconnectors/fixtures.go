// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/enterpriseconnectors"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

func init() {
	// Note that for these fixtures the credentials are configured with the specific policy parameters through dpanel.
	testing.AddFixture(&testing.Fixture{
		Name: "ashGaiaSignedInProdPolicyWPEnabledAllowExtra",
		Desc: "Fixture that allows usage of ash, with a gaia login with production policy and enabled WebProtect scanning which allows immediate file transfers, large and encrypted files",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: createFixtureByPool(
			enterpriseconnectors.AshAccount3VarName,
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "ashGaiaSignedInProdPolicyWPEnabledBlockExtra",
		Desc: "Fixture that allows usage of ash, with a gaia login with production policy and enabled WebProtect scanning which blocks immediate file transfers, large and encrypted files",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: createFixtureByPool(
			enterpriseconnectors.AshAccount1VarName,
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "ashGaiaSignedInProdPolicyWPDisabled",
		Desc: "Fixture that allows usage of ash, with a gaia login with production policy and disabled WebProtect scanning",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: createFixtureByPool(
			enterpriseconnectors.AshAccount2VarName,
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosGaiaSignedInProdPolicyWPEnabledAllowExtra",
		Desc: "Fixture that allows usage of Lacros, with a gaia login with production policy and enabled WebProtect scanning which allows immediate file transfers, large and encrypted files",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: CreateFixture(
			"enterpriseconnectors.lacros_username3",
			"enterpriseconnectors.lacros_password3",
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"enterpriseconnectors.lacros_username3",
			"enterpriseconnectors.lacros_password3",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosGaiaSignedInProdPolicyWPEnabledBlockExtra",
		Desc: "Fixture that allows usage of Lacros, with a gaia login with production policy and enabled WebProtect scanning which blocks immediate file transfers, large and encrypted files",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: CreateFixture(
			"enterpriseconnectors.lacros_username1",
			"enterpriseconnectors.lacros_password1",
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"enterpriseconnectors.lacros_username1",
			"enterpriseconnectors.lacros_password1",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "lacrosGaiaSignedInProdPolicyWPDisabled",
		Desc: "Fixture that allows usage of Lacros, with a gaia login with production policy and disabled WebProtect scanning",
		Contacts: []string{
			"sseckler@google.com",
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
		},
		BugComponent: "b:1240978",
		Impl: CreateFixture(
			"enterpriseconnectors.lacros_username2",
			"enterpriseconnectors.lacros_password2",
		),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 3*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"enterpriseconnectors.lacros_username2",
			"enterpriseconnectors.lacros_password2",
		},
	})
}

func CreateFixture(user, pw string) testing.FixtureImpl {
	return chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		username := s.RequiredVar(user)
		password := s.RequiredVar(pw)
		return lacrosfixt.NewConfig(
			lacrosfixt.ChromeOptions(
				chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
				chrome.ProdPolicy(),
				chrome.EnableFeatures("FileTransferEnterpriseConnector", "FileTransferEnterpriseConnectorUI", "NewFilesPolicyUX"),
				chrome.ExtraArgs("--disable-search-engine-choice-screen"),
			),
		).Opts()
	})
}

func createFixtureByPool(account string) testing.FixtureImpl {
	return chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return lacrosfixt.NewConfig(
			lacrosfixt.ChromeOptions(
				chrome.GAIALoginPool(dma.CredsFromPool(account)),
				chrome.ProdPolicy(),
				chrome.EnableFeatures("FileTransferEnterpriseConnector", "FileTransferEnterpriseConnectorUI", "NewFilesPolicyUX"),
				chrome.ExtraArgs("--disable-search-engine-choice-screen"),
			),
		).Opts()
	})
}
