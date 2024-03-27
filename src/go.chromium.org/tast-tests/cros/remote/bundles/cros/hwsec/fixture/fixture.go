// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/hwsec/util"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	crossVersionBackupSetUpTimeout    = 1 * time.Minute
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
	return params
}

type cleanupFunc func(context.Context) error

type backupFixtImpl struct {
	cleanup cleanupFunc
}

type backupFixture struct {
	WebAuthnURL string
}

// SetUp backs up the login data for TearDown
func (f *backupFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	// Soft clear the TPM before preparing the DUT.
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to reset TPM or system states: ", err)
	}

	// Create backup data to recover state later.
	backupPath := "/mnt/stateful_partition/unencrypted/tpm2-simulator/backup_data.tar.xz"
	if err := helper.SaveLoginData(ctx, backupPath, true /*includeTpm*/); err != nil {
		s.Fatal("Failed to backup login data: ", err)
	}
	f.cleanup = func(ctx context.Context) error {
		// Load back the origin login data after the test.
		if err := helper.LoadLoginData(ctx, backupPath, true /*includeTpm*/, false /*resumeDaemos*/); err != nil {
			return errors.Wrap(err, "failed to load login data")
		}
		if err := s.DUT().Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}

		// The first reboot will change the mount-encrypted key, so we will need to restore the data twice to ensure the data will be preserved after the next reboot.
		if err := helper.LoadLoginData(ctx, backupPath, true /*includeTpm*/, false /*resumeDaemos*/); err != nil {
			return errors.Wrap(err, "failed to load login data")
		}
		if err := s.DUT().Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
		if err := helper.RemoveAll(ctx, backupPath); err != nil {
			return errors.Wrapf(err, "failed to clean up %q", backupPath)
		}
		return nil
	}
	return backupFixture{}
}

// TearDown restores the login data backed up by SetUp
func (f *backupFixtImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cleanup(ctx); err != nil {
		s.Fatal("Failed to cleanup: ", err)
	}
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
	s        *testing.FixtState
}

type crossVersionFixtParamVal struct {
	dataPrefix string
	dataDir    string
}

// CrossVersionLoginFixture contains the config list for the login data used in cross version testing.
type CrossVersionLoginFixture struct {
	ConfigList  []util.CrossVersionLoginConfig
	WebAuthnURL string
	DataPath    string
}

// SetUp loads the data of the milestone specified in the crossVersionFixtImpl.dataPrefix
func (f *crossVersionFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	parentData := s.ParentValue().(backupFixture)
	f.s = s
	paramVal := s.Param().(crossVersionFixtParamVal)

	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	tmpDirData, err := cmdRunner.Run(ctx, "mktemp", "-d", "/mnt/stateful_partition/unencrypted/tpm2-simulator/cross_version_XXXXXX")
	if err != nil {
		s.Fatal("Failed to create tmp dir: ", err)
	}

	tmpDir := strings.TrimSpace(string(tmpDirData))

	var dataPath string
	var configPath string

	dataName := fmt.Sprintf("%s/%s_data.tar.gz", paramVal.dataDir, paramVal.dataPrefix)
	configName := fmt.Sprintf("%s/%s_config.json", paramVal.dataDir, paramVal.dataPrefix)
	dataPath = filepath.Join(tmpDir, "data.tar.gz")
	configPath = filepath.Join(tmpDir, "config.json")

	if _, err := linuxssh.PutFiles(
		ctx, s.DUT().Conn(), map[string]string{
			s.DataPath(dataName): dataPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", dataPath, err)
	}

	if _, err := linuxssh.PutFiles(
		ctx, s.DUT().Conn(), map[string]string{
			s.DataPath(configName): configPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to send data to remote data path %v: %v", configPath, err)
	}

	configJSON, err := helper.ReadFile(ctx, configPath)
	if err != nil {
		s.Fatalf("Failed to read %q: %v", configPath, err)
	}
	var configList []util.CrossVersionLoginConfig
	if err := json.Unmarshal(configJSON, &configList); err != nil {
		s.Fatal("Failed to read json: ", err)
	}

	if err := helper.LoadLoginData(ctx, dataPath, true /*includeTpm*/, false /*resumeDaemos*/); err != nil {
		s.Fatal("Failed to load login data: ", err)
	}
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	// The first reboot will change the mount-encrypted key, so we will need to restore the data twice to ensure the data will be preserved after the next reboot.
	if err := helper.LoadLoginData(ctx, dataPath, true /*includeTpm*/, false /*resumeDaemos*/); err != nil {
		s.Fatal("Failed to load login data: ", err)
	}
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	f.dataPath = dataPath
	return &CrossVersionLoginFixture{
		ConfigList:  configList,
		WebAuthnURL: parentData.WebAuthnURL,
		DataPath:    dataPath,
	}
}

func (f *crossVersionFixtImpl) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *crossVersionFixtImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crossVersionFixtImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crossVersionFixtImpl) Reset(ctx context.Context) error {
	return nil
}
