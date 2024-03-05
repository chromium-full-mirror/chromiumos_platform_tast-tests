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

	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/errors"
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
	return testing.FixtureParam{
		Name: fmt.Sprintf("%s_r%d", hsmName, milestone),
		Val: crossVersionFixtParamVal{
			dataPrefix: dataPrefix,
			useCurrent: false,
		},
		ExtraData: []string{
			"cross_version_login/" + dataPrefix + "_config.json",
			"cross_version_login/" + dataPrefix + "_data.tar.gz",
		},
	}
}

func genXverFixtParams() (params []testing.FixtureParam) {
	// Data prefixes for cross version with TPM2.0
	tpm2DataPrefixes := map[int]string{
		88:  "R88-13597.108.0-custombuild20220717_betty_20220719",
		89:  "R89-13729.85.0-custombuild20220715_betty_20220719",
		90:  "R90-13816.106.0-custombuild20220712_betty_20220719",
		91:  "R91-13904.98.0-custombuild20220712_betty_20220719",
		92:  "R92-13982.89.0-custombuild20220712_betty_20220719",
		93:  "R93-14092.106.0-custombuild20220713_betty_20220719",
		94:  "R94-14150.592.0-custombuild20220714_betty_20220720",
		96:  "R96-14268.94.0-custombuild20220714_betty_20220719",
		97:  "R97-14324.81.0-custombuild20220715_betty_20220719",
		98:  "R98-14388.65.0-custombuild20220715_betty_20220719",
		99:  "R99-14469.76.0-custombuild20220717_betty_20220719",
		100: "R100-14526.122.0-custombuild20220718_betty_20220719",
		101: "R101-14588.134.0-custombuild20220718_betty_20220719",
		102: "R102-14695.114.0-custombuild20220718_betty_20220719",
		103: "R103-14816.99.0_betty_20220712",
		104: "R104-14909.132.0_betty_20221202",
		105: "R105-14989.107.0_betty_20221202",
		106: "R106-15054.114.0_betty_20221129",
		107: "R107-15117.112.0_betty_20221129",
		108: "R108-15183.69.0_betty_20221221",
		109: "R109-15236.82.0_novato_20230216",
		110: "R110-15278.66.0_novato_20230216",
		111: "R111-15329.61.0_novato_20230329",
		112: "R112-15359.49.0_novato_20230410",
		113: "R113-15393.65.0_novato_20230627",
		114: "R114-15437.60.0_novato_20230627",
		115: "R115-15474.84.0_novato_20230830",
		116: "R116-15509.71.0_novato_20230830",
		117: "R117-15572.63.0_novato_20231031",
		118: "R118-15604.33.0_novato_20231123",
		119: "R119-15633.69.0_amd64-generic_20231212",
	}
	// Data prefixes for cross version with TPM dynamic
	tpmDynamicDataPrefixes := map[int]string{
		96:  "R96-14268.94.0-custombuild20220715_reven-vmtest_20220719",
		97:  "R97-14324.81.0-custombuild20220716_reven-vmtest_20220719",
		98:  "R98-14388.65.0-custombuild20220719_reven-vmtest_20220719",
		99:  "R99-14469.76.0-custombuild20220718_reven-vmtest_20220719",
		100: "R100-14526.122.0-custombuild20220718_reven-vmtest_20220719",
		101: "R101-14588.134.0-custombuild20220718_reven-vmtest_20220719",
		102: "R102-14695.114.0-custombuild20220718_reven-vmtest_20220719",
		103: "R103-14816.99.0_reven-vmtest_20220712",
		104: "R104-14909.132.0_reven-vmtest_20221202",
		105: "R105-14989.108.0_reven-vmtest_20221202",
		106: "R106-15054.114.0_reven-vmtest_20221129",
		107: "R107-15117.112.0_reven-vmtest_20221129",
		108: "R108-15183.69.0_reven-vmtest_20221221",
		109: "R109-15236.82.0_reven-vmtest_20230216",
		110: "R110-15278.66.0_reven-vmtest_20230216",
		111: "R111-15329.61.0_reven-vmtest_20230329",
		112: "R112-15359.49.0_reven-vmtest_20230410",
		113: "R113-15393.65.0_reven-vmtest_20230627",
		114: "R114-15437.60.0_reven-vmtest_20230627",
		115: "R115-15474.84.0_reven-vmtest_20230830",
		116: "R116-15509.71.0_reven-vmtest_20230830",
		117: "R117-15572.63.0_reven-vmtest_20231031",
		118: "R118-15604.60.0_reven-vmtest_20231123",
		119: "R119-15633.69.0_reven-vmtest_20231212",
	}
	// Data prefixes for cross version with Ti50 emulator
	ti50DataPrefixes := map[int]string{
		112: "R112-15359.49.0_betty_20230410",
		113: "R113-15393.65.0_betty_20230627",
		114: "R114-15437.60.0_betty_20230627",
		115: "R115-15474.84.0_betty_20230830",
		116: "R116-15509.71.0_betty_20230830",
		117: "R117-15572.63.0_betty_20231031",
		118: "R118-15604.60.0_betty_20231123",
		119: "R119-15633.69.0_betty_20231212",
	}

	for milestone, dataPrefix := range tpm2DataPrefixes {
		params = append(params, xverFixtParamFactory("tpm2", milestone, dataPrefix))
	}
	for milestone, dataPrefix := range tpmDynamicDataPrefixes {
		params = append(params, xverFixtParamFactory("tpm_dynamic", milestone, dataPrefix))
	}
	for milestone, dataPrefix := range ti50DataPrefixes {
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
		dataName := fmt.Sprintf("cross_version_login/%s_data.tar.gz", paramVal.dataPrefix)
		configName := fmt.Sprintf("cross_version_login/%s_config.json", paramVal.dataPrefix)
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
