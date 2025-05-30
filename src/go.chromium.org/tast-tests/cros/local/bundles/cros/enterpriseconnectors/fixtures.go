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
}

func CreateFixture(user, pw string) testing.FixtureImpl {
	return chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		username := s.RequiredVar(user)
		password := s.RequiredVar(pw)
		return []chrome.Option{
			chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
			chrome.ProdPolicy(),
			chrome.EnableFeatures("FileTransferEnterpriseConnector", "FileTransferEnterpriseConnectorUI", "NewFilesPolicyUX"),
			chrome.ExtraArgs("--disable-search-engine-choice-screen"),
		}, nil

	})
}

func createFixtureByPool(account string) testing.FixtureImpl {
	return chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.GAIALoginPool(dma.CredsFromPool(account)),
			chrome.ProdPolicy(),
			chrome.EnableFeatures("FileTransferEnterpriseConnector", "FileTransferEnterpriseConnectorUI", "NewFilesPolicyUX"),
			chrome.ExtraArgs("--disable-search-engine-choice-screen"),
		}, nil
	})
}
