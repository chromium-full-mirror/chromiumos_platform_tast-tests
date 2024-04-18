// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	crossVersionBackupSetUpTimeout    = 90 * time.Second
	crossVersionBackupResetTimeout    = 30 * time.Second
	crossVersionBackupTearDownTimeout = 1 * time.Minute
	crossVersionSetUpTimeout          = 1 * time.Minute
	crossVersionCurrentSetUpTimeout   = crossVersionSetUpTimeout + 2*time.Minute // X-ver setup + extra preparation time for `useCurrent`.
	crossVersionResetTimeout          = 30 * time.Second
	crossVersionTearDownTimeout       = 30 * time.Second
)

var webauthnData = []string{
	"webauthn.html",
	"bundle.js",
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionBackup",
		Desc: "Backs up for cross version testing",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		// ChromeOS > Platform > baseOS > Hardware Security > HwSec AP
		BugComponent:    "b:1188704",
		SetUpTimeout:    crossVersionBackupSetUpTimeout,
		ResetTimeout:    crossVersionBackupResetTimeout,
		TearDownTimeout: crossVersionBackupTearDownTimeout,
		Impl:            &backupFixtImpl{},
		Data:            webauthnData,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersion",
		Desc: "Loads the cross version data",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		// ChromeOS > Platform > baseOS > Hardware Security > HwSec AP
		BugComponent:    "b:1188704",
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl:            &crossVersionFixtImpl{},
		Params:          genXverFixtParams(),
	})
}

func xverFixtParamFactory(hsmName string, milestone int, dataPrefix string) testing.FixtureParam {
	dataDir := "cross_version_login/" + hsmName
	return testing.FixtureParam{
		Name: fmt.Sprintf("%s_r%d", hsmName, milestone),
		Val: crossVersionFixtParamVal{
			dataPrefix: dataPrefix,
			dataDir:    dataDir,
			useCurrent: false,
		},
		ExtraData: []string{
			dataDir + "/" + dataPrefix + "_config.json",
			dataDir + "/" + dataPrefix + "_data.tar.gz",
		},
	}
}

func genXverFixtParams() (params []testing.FixtureParam) {
	for milestone, dataPrefix := range hwsec.Tpm2DataPrefixes {
		params = append(params, xverFixtParamFactory("tpm2", milestone, dataPrefix))
	}
	for milestone, dataPrefix := range hwsec.TpmDynamicDataPrefixes {
		params = append(params, xverFixtParamFactory("tpm_dynamic", milestone, dataPrefix))
	}
	for milestone, dataPrefix := range hwsec.Ti50DataPrefixes {
		params = append(params, xverFixtParamFactory("ti50", milestone, dataPrefix))
	}
	// Param for testing loading the data from the current device to itself
	params = append(params, testing.FixtureParam{
		Name: "current",
		Val: crossVersionFixtParamVal{
			useCurrent: true,
		},
	})
	return params
}

type cleanupFunc func(context.Context) error

type backupFixtImpl struct {
	cleanup        cleanupFunc
	webauthnServer *u2fd.WebAuthnHTTPServer
}

type backupFixture struct {
	WebAuthnURL string
}

// SetUp backs up the login data for TearDown
func (f *backupFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	// Soft clear the TPM before preparing the DUT.
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to reset TPM or system states: ", err)
	}

	tmpDir, err := ioutil.TempDir("", "cross_version_login")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	// Create backup data to recover state later.
	backupPath := filepath.Join(tmpDir, "backup_data.tar.xz")
	if err := helper.SaveLoginData(ctx, backupPath, true /*includeTpm*/); err != nil {
		s.Fatal("Failed to backup login data: ", err)
	}
	f.webauthnServer = u2fd.NewWebAuthnHTTPServer(ctx, s.DataFileSystem())
	f.cleanup = func(ctx context.Context) error {
		// Load back the origin login data after the test.
		if err := helper.LoadLoginData(ctx, backupPath, true /*includeTpm*/, true /*resumeDaemos*/); err != nil {
			return errors.Wrap(err, "failed to load login data")
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			return errors.Wrapf(err, "failed to clean up %q", tmpDir)
		}
		return nil
	}
	return backupFixture{
		WebAuthnURL: f.webauthnServer.URL + "/webauthn.html",
	}
}

// TearDown restores the login data backed up by SetUp
func (f *backupFixtImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Fatal("Failed to cleanup: ", err)
	}
	f.webauthnServer.Close(ctx)
}

func (f *backupFixtImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *backupFixtImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *backupFixtImpl) Reset(ctx context.Context) error {
	return nil
}

type crossVersionFixtImpl struct {
	dataPath string
}

type crossVersionFixtParamVal struct {
	dataPrefix string
	dataDir    string
	useCurrent bool
}

// CrossVersionLoginFixture contains the config list for the login data used in cross version testing.
type CrossVersionLoginFixture struct {
	ConfigList  []util.CrossVersionLoginConfig
	WebAuthnURL string
}

// SetUp loads the data of the milestone specified in the crossVersionFixtImpl.dataPrefix
func (f *crossVersionFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	parentData := s.ParentValue().(backupFixture)
	paramVal := s.Param().(crossVersionFixtParamVal)

	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	tmpDir, err := ioutil.TempDir("", "cross_version_login")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}

	var dataPath string
	var configPath string

	if paramVal.useCurrent {
		dataPath = filepath.Join(tmpDir, "data.tar.gz")
		configPath = filepath.Join(tmpDir, "config.json")
		s.Log("Preparing login data of current version")
		if err := util.PrepareCrossVersionLoginData(ctx, s.Logf, helper.CmdHelper, dataPath, configPath, parentData.WebAuthnURL); err != nil {
			s.Fatal("Failed to prepare login data for current version: ", err)
		}
	} else {
		dataName := fmt.Sprintf("%s/%s_data.tar.gz", paramVal.dataDir, paramVal.dataPrefix)
		configName := fmt.Sprintf("%s/%s_config.json", paramVal.dataDir, paramVal.dataPrefix)
		dataPath = s.DataPath(dataName)
		configPath = s.DataPath(configName)
	}

	configJSON, err := ioutil.ReadFile(configPath)
	if err != nil {
		s.Fatalf("Failed to read %q: %v", configPath, err)
	}
	var configList []util.CrossVersionLoginConfig
	if err := json.Unmarshal(configJSON, &configList); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	if err := helper.LoadLoginData(ctx, dataPath, true /*includeTpm*/, true /*resumeDaemos*/); err != nil {
		s.Fatal("Failed to load login data: ", err)
	}

	f.dataPath = dataPath
	return &CrossVersionLoginFixture{
		ConfigList:  configList,
		WebAuthnURL: parentData.WebAuthnURL,
	}
}

func (f *crossVersionFixtImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *crossVersionFixtImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crossVersionFixtImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crossVersionFixtImpl) Reset(ctx context.Context) error {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to create hwsec local helper")
	}
	if err := helper.LoadLoginData(ctx, f.dataPath, true /*includeTpm*/, true /*resumeDaemos*/); err != nil {
		return errors.Wrap(err, "failed to load login data")
	}
	return nil
}
