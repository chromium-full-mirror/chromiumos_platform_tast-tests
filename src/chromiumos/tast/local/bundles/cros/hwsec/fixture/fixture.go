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

	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/hwsec/util"
	hwseclocal "chromiumos/tast/local/hwsec"
	"chromiumos/tast/testing"
)

const (
	crossVersionBackupSetUpTimeout    = 1 * time.Minute
	crossVersionBackupResetTimeout    = 30 * time.Second
	crossVersionBackupTearDownTimeout = 1 * time.Minute
	crossVersionSetUpTimeout          = 1 * time.Minute
	crossVersionResetTimeout          = 30 * time.Second
	crossVersionTearDownTimeout       = 30 * time.Second
)

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
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionCurrent",
		Desc: "Loads the data from the current device to itself",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "",
			useCurrent: true,
		},
	})
}

type cleanupFunc func(context.Context) error

type backupFixtImpl struct {
	cleanup cleanupFunc
}

// SetUp backs up the login data for TearDown
func (f *backupFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	daemonController := helper.DaemonController()

	tmpDir, err := ioutil.TempDir("", "cross_version_login")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	// Create backup data to recover state later.
	backupPath := filepath.Join(tmpDir, "backup_data.tar.xz")
	if err := hwseclocal.SaveLoginData(ctx, daemonController, backupPath, true /*includeTpm*/); err != nil {
		s.Fatal("Failed to backup login data: ", err)
	}
	f.cleanup = func(ctx context.Context) error {
		// Load back the origin login data after the test.
		if err := hwseclocal.LoadLoginData(ctx, daemonController, backupPath, true /*includeTpm*/); err != nil {
			return errors.Wrap(err, "failed to load login data")
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			return errors.Wrapf(err, "failed to clean up %q", tmpDir)
		}
		return nil
	}
	return nil
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
	dataPrefix string
	useCurrent bool
}

// CrossVersionLoginFixture contains the config list for the login data used in cross version testing.
type CrossVersionLoginFixture struct {
	ConfigList []util.CrossVersionLoginConfig
}

// SetUp loads the data of the milestone specified in the crossVersionFixtImpl.dataPrefix
func (f *crossVersionFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	daemonController := helper.DaemonController()
	cryptohome := helper.CryptohomeClient()

	tmpDir, err := ioutil.TempDir("", "cross_version_login")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}

	var dataPath string
	var configPath string

	if f.useCurrent {
		dataPath = filepath.Join(tmpDir, "data.tar.gz")
		configPath = filepath.Join(tmpDir, "config.json")
		s.Log("Preparing login data of current version")
		if err := util.PrepareCrossVersionLoginData(ctx, s.Logf, cryptohome, daemonController, dataPath, configPath); err != nil {
			s.Fatal("Failed to prepare login data for current version: ", err)
		}
	} else {
		dataName := fmt.Sprintf("cross_version_login/%s_data.tar.gz", f.dataPrefix)
		configName := fmt.Sprintf("cross_version_login/%s_config.json", f.dataPrefix)
		dataPath = s.DataPath(dataName)
		configPath = s.DataPath(configName)

	}

	configJSON, err := ioutil.ReadFile(configPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q", configPath)
	}
	var configList []util.CrossVersionLoginConfig
	if err := json.Unmarshal(configJSON, &configList); err != nil {
		return errors.Wrap(err, "failed to read json")
	}

	if err := hwseclocal.LoadLoginData(ctx, daemonController, dataPath, true /*includeTpm*/); err != nil {
		return errors.Wrap(err, "failed to load login data")
	}
	return &CrossVersionLoginFixture{
		ConfigList: configList,
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
