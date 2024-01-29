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
		Name: "crossVersionCurrent",
		Desc: "Loads the data from the current device to itself",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionCurrentSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "",
			useCurrent: true,
		},
	})

	// Fixtures of cross version with Ti50 emulator
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R112",
		Desc: "Loads the data of milestone R112 from the Ti50 emulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R112-15359.49.0_betty_20230410",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R112-15359.49.0_betty_20230410_config.json",
			"cross_version_login/R112-15359.49.0_betty_20230410_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R113",
		Desc: "Loads the data of milestone R113 from the Ti50 emulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R113-15393.65.0_betty_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R113-15393.65.0_betty_20230627_config.json",
			"cross_version_login/R113-15393.65.0_betty_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R114",
		Desc: "Loads the data of milestone R114 from the Ti50 emulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R114-15437.60.0_betty_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R114-15437.60.0_betty_20230627_config.json",
			"cross_version_login/R114-15437.60.0_betty_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R115",
		Desc: "Loads the data of milestone R115 from the Ti50 emulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R115-15474.84.0_betty_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R115-15474.84.0_betty_20230830_config.json",
			"cross_version_login/R115-15474.84.0_betty_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R116",
		Desc: "Loads the data of milestone R116 from the Ti50 emulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R116-15509.71.0_betty_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R116-15509.71.0_betty_20230830_config.json",
			"cross_version_login/R116-15509.71.0_betty_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R117",
		Desc: "Loads the data of milestone R117 from the Ti50 simulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R117-15572.63.0_betty_20231031",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R117-15572.63.0_betty_20231031_config.json",
			"cross_version_login/R117-15572.63.0_betty_20231031_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R118",
		Desc: "Loads the data of milestone R118 from the Ti50 simulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R118-15604.60.0_betty_20231123",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R118-15604.60.0_betty_20231123_config.json",
			"cross_version_login/R118-15604.60.0_betty_20231123_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTi50R119",
		Desc: "Loads the data of milestone R119 from the Ti50 simulator device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R119-15633.69.0_betty_20231212",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R119-15633.69.0_betty_20231212_config.json",
			"cross_version_login/R119-15633.69.0_betty_20231212_data.tar.gz",
		},
	})

	// Fixtures of cross version with TPM2.0
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R88",
		Desc: "Loads the data of milestone R88 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R88-13597.108.0-custombuild20220717_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R88-13597.108.0-custombuild20220717_betty_20220719_config.json",
			"cross_version_login/R88-13597.108.0-custombuild20220717_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R89",
		Desc: "Loads the data of milestone R89 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R89-13729.85.0-custombuild20220715_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R89-13729.85.0-custombuild20220715_betty_20220719_config.json",
			"cross_version_login/R89-13729.85.0-custombuild20220715_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R90",
		Desc: "Loads the data of milestone R90 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R90-13816.106.0-custombuild20220712_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R90-13816.106.0-custombuild20220712_betty_20220719_config.json",
			"cross_version_login/R90-13816.106.0-custombuild20220712_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R91",
		Desc: "Loads the data of milestone R91 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R91-13904.98.0-custombuild20220712_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R91-13904.98.0-custombuild20220712_betty_20220719_config.json",
			"cross_version_login/R91-13904.98.0-custombuild20220712_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R92",
		Desc: "Loads the data of milestone R92 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R92-13982.89.0-custombuild20220712_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R92-13982.89.0-custombuild20220712_betty_20220719_config.json",
			"cross_version_login/R92-13982.89.0-custombuild20220712_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R93",
		Desc: "Loads the data of milestone R93 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R93-14092.106.0-custombuild20220713_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R93-14092.106.0-custombuild20220713_betty_20220719_config.json",
			"cross_version_login/R93-14092.106.0-custombuild20220713_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R94",
		Desc: "Loads the data of milestone R94 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R94-14150.592.0-custombuild20220714_betty_20220720",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R94-14150.592.0-custombuild20220714_betty_20220720_config.json",
			"cross_version_login/R94-14150.592.0-custombuild20220714_betty_20220720_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R96",
		Desc: "Loads the data of milestone R96 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R96-14268.94.0-custombuild20220714_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R96-14268.94.0-custombuild20220714_betty_20220719_config.json",
			"cross_version_login/R96-14268.94.0-custombuild20220714_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R97",
		Desc: "Loads the data of milestone R97 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R97-14324.81.0-custombuild20220715_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R97-14324.81.0-custombuild20220715_betty_20220719_config.json",
			"cross_version_login/R97-14324.81.0-custombuild20220715_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R98",
		Desc: "Loads the data of milestone R98 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R98-14388.65.0-custombuild20220715_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R98-14388.65.0-custombuild20220715_betty_20220719_config.json",
			"cross_version_login/R98-14388.65.0-custombuild20220715_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R99",
		Desc: "Loads the data of milestone R99 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R99-14469.76.0-custombuild20220717_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R99-14469.76.0-custombuild20220717_betty_20220719_config.json",
			"cross_version_login/R99-14469.76.0-custombuild20220717_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R100",
		Desc: "Loads the data of milestone R100 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R100-14526.122.0-custombuild20220718_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R100-14526.122.0-custombuild20220718_betty_20220719_config.json",
			"cross_version_login/R100-14526.122.0-custombuild20220718_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R101",
		Desc: "Loads the data of milestone R101 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R101-14588.134.0-custombuild20220718_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R101-14588.134.0-custombuild20220718_betty_20220719_config.json",
			"cross_version_login/R101-14588.134.0-custombuild20220718_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R102",
		Desc: "Loads the data of milestone R102 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R102-14695.114.0-custombuild20220718_betty_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R102-14695.114.0-custombuild20220718_betty_20220719_config.json",
			"cross_version_login/R102-14695.114.0-custombuild20220718_betty_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R103",
		Desc: "Loads the data of milestone R103 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R103-14816.99.0_betty_20220712",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R103-14816.99.0_betty_20220712_config.json",
			"cross_version_login/R103-14816.99.0_betty_20220712_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R104",
		Desc: "Loads the data of milestone R104 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R104-14909.132.0_betty_20221202",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R104-14909.132.0_betty_20221202_config.json",
			"cross_version_login/R104-14909.132.0_betty_20221202_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R105",
		Desc: "Loads the data of milestone R105 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R105-14989.107.0_betty_20221202",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R105-14989.107.0_betty_20221202_config.json",
			"cross_version_login/R105-14989.107.0_betty_20221202_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R106",
		Desc: "Loads the data of milestone R106 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R106-15054.114.0_betty_20221129",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R106-15054.114.0_betty_20221129_config.json",
			"cross_version_login/R106-15054.114.0_betty_20221129_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R107",
		Desc: "Loads the data of milestone R107 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R107-15117.112.0_betty_20221129",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R107-15117.112.0_betty_20221129_config.json",
			"cross_version_login/R107-15117.112.0_betty_20221129_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R108",
		Desc: "Loads the data of milestone R108 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R108-15183.69.0_betty_20221221",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R108-15183.69.0_betty_20221221_config.json",
			"cross_version_login/R108-15183.69.0_betty_20221221_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R109",
		Desc: "Loads the data of milestone R109 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R109-15236.82.0_novato_20230216",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R109-15236.82.0_novato_20230216_config.json",
			"cross_version_login/R109-15236.82.0_novato_20230216_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R110",
		Desc: "Loads the data of milestone R110 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R110-15278.66.0_novato_20230216",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R110-15278.66.0_novato_20230216_config.json",
			"cross_version_login/R110-15278.66.0_novato_20230216_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R111",
		Desc: "Loads the data of milestone R111 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R111-15329.61.0_novato_20230329",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R111-15329.61.0_novato_20230329_config.json",
			"cross_version_login/R111-15329.61.0_novato_20230329_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R112",
		Desc: "Loads the data of milestone R112 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R112-15359.49.0_novato_20230410",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R112-15359.49.0_novato_20230410_config.json",
			"cross_version_login/R112-15359.49.0_novato_20230410_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R113",
		Desc: "Loads the data of milestone R113 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R113-15393.65.0_novato_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R113-15393.65.0_novato_20230627_config.json",
			"cross_version_login/R113-15393.65.0_novato_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R114",
		Desc: "Loads the data of milestone R114 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R114-15437.60.0_novato_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R114-15437.60.0_novato_20230627_config.json",
			"cross_version_login/R114-15437.60.0_novato_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R115",
		Desc: "Loads the data of milestone R115 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R115-15474.84.0_novato_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R115-15474.84.0_novato_20230830_config.json",
			"cross_version_login/R115-15474.84.0_novato_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R116",
		Desc: "Loads the data of milestone R116 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R116-15509.71.0_novato_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R116-15509.71.0_novato_20230830_config.json",
			"cross_version_login/R116-15509.71.0_novato_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R117",
		Desc: "Loads the data of milestone R117 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R117-15572.63.0_novato_20231031",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R117-15572.63.0_novato_20231031_config.json",
			"cross_version_login/R117-15572.63.0_novato_20231031_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R118",
		Desc: "Loads the data of milestone R118 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R118-15604.33.0_novato_20231123",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R118-15604.33.0_novato_20231123_config.json",
			"cross_version_login/R118-15604.33.0_novato_20231123_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpm2R119",
		Desc: "Loads the data of milestone R119 from the tpm2 device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R119-15633.69.0_amd64-generic_20231212",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R119-15633.69.0_amd64-generic_20231212_config.json",
			"cross_version_login/R119-15633.69.0_amd64-generic_20231212_data.tar.gz",
		},
	})

	// Fixtures of cross version with TPM dynamic
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR96",
		Desc: "Loads the data of milestone R96 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R96-14268.94.0-custombuild20220715_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R96-14268.94.0-custombuild20220715_reven-vmtest_20220719_config.json",
			"cross_version_login/R96-14268.94.0-custombuild20220715_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR97",
		Desc: "Loads the data of milestone R97 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R97-14324.81.0-custombuild20220716_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R97-14324.81.0-custombuild20220716_reven-vmtest_20220719_config.json",
			"cross_version_login/R97-14324.81.0-custombuild20220716_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR98",
		Desc: "Loads the data of milestone R98 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R98-14388.65.0-custombuild20220719_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R98-14388.65.0-custombuild20220719_reven-vmtest_20220719_config.json",
			"cross_version_login/R98-14388.65.0-custombuild20220719_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR99",
		Desc: "Loads the data of milestone R99 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R99-14469.76.0-custombuild20220718_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R99-14469.76.0-custombuild20220718_reven-vmtest_20220719_config.json",
			"cross_version_login/R99-14469.76.0-custombuild20220718_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR100",
		Desc: "Loads the data of milestone R100 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R100-14526.122.0-custombuild20220718_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R100-14526.122.0-custombuild20220718_reven-vmtest_20220719_config.json",
			"cross_version_login/R100-14526.122.0-custombuild20220718_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR101",
		Desc: "Loads the data of milestone R101 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R101-14588.134.0-custombuild20220718_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R101-14588.134.0-custombuild20220718_reven-vmtest_20220719_config.json",
			"cross_version_login/R101-14588.134.0-custombuild20220718_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR102",
		Desc: "Loads the data of milestone R102 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R102-14695.114.0-custombuild20220718_reven-vmtest_20220719",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R102-14695.114.0-custombuild20220718_reven-vmtest_20220719_config.json",
			"cross_version_login/R102-14695.114.0-custombuild20220718_reven-vmtest_20220719_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR103",
		Desc: "Loads the data of milestone R103 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R103-14816.99.0_reven-vmtest_20220712",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R103-14816.99.0_reven-vmtest_20220712_config.json",
			"cross_version_login/R103-14816.99.0_reven-vmtest_20220712_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR104",
		Desc: "Loads the data of milestone R104 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R104-14909.132.0_reven-vmtest_20221202",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R104-14909.132.0_reven-vmtest_20221202_config.json",
			"cross_version_login/R104-14909.132.0_reven-vmtest_20221202_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR105",
		Desc: "Loads the data of milestone R105 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R105-14989.108.0_reven-vmtest_20221202",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R105-14989.108.0_reven-vmtest_20221202_config.json",
			"cross_version_login/R105-14989.108.0_reven-vmtest_20221202_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR106",
		Desc: "Loads the data of milestone R106 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R106-15054.114.0_reven-vmtest_20221129",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R106-15054.114.0_reven-vmtest_20221129_config.json",
			"cross_version_login/R106-15054.114.0_reven-vmtest_20221129_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR107",
		Desc: "Loads the data of milestone R107 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R107-15117.112.0_reven-vmtest_20221129",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R107-15117.112.0_reven-vmtest_20221129_config.json",
			"cross_version_login/R107-15117.112.0_reven-vmtest_20221129_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR108",
		Desc: "Loads the data of milestone R108 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R108-15183.69.0_reven-vmtest_20221221",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R108-15183.69.0_reven-vmtest_20221221_config.json",
			"cross_version_login/R108-15183.69.0_reven-vmtest_20221221_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR109",
		Desc: "Loads the data of milestone R109 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R109-15236.82.0_reven-vmtest_20230216",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R109-15236.82.0_reven-vmtest_20230216_config.json",
			"cross_version_login/R109-15236.82.0_reven-vmtest_20230216_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR110",
		Desc: "Loads the data of milestone R110 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R110-15278.66.0_reven-vmtest_20230216",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R110-15278.66.0_reven-vmtest_20230216_config.json",
			"cross_version_login/R110-15278.66.0_reven-vmtest_20230216_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR111",
		Desc: "Loads the data of milestone R111 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R111-15329.61.0_reven-vmtest_20230329",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R111-15329.61.0_reven-vmtest_20230329_config.json",
			"cross_version_login/R111-15329.61.0_reven-vmtest_20230329_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR112",
		Desc: "Loads the data of milestone R112 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R112-15359.49.0_reven-vmtest_20230410",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R112-15359.49.0_reven-vmtest_20230410_config.json",
			"cross_version_login/R112-15359.49.0_reven-vmtest_20230410_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR113",
		Desc: "Loads the data of milestone R113 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R113-15393.65.0_reven-vmtest_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R113-15393.65.0_reven-vmtest_20230627_config.json",
			"cross_version_login/R113-15393.65.0_reven-vmtest_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR114",
		Desc: "Loads the data of milestone R114 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R114-15437.60.0_reven-vmtest_20230627",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R114-15437.60.0_reven-vmtest_20230627_config.json",
			"cross_version_login/R114-15437.60.0_reven-vmtest_20230627_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR115",
		Desc: "Loads the data of milestone R115 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R115-15474.84.0_reven-vmtest_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R115-15474.84.0_reven-vmtest_20230830_config.json",
			"cross_version_login/R115-15474.84.0_reven-vmtest_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR116",
		Desc: "Loads the data of milestone R116 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R116-15509.71.0_reven-vmtest_20230830",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R116-15509.71.0_reven-vmtest_20230830_config.json",
			"cross_version_login/R116-15509.71.0_reven-vmtest_20230830_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR117",
		Desc: "Loads the data of milestone R117 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R117-15572.63.0_reven-vmtest_20231031",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R117-15572.63.0_reven-vmtest_20231031_config.json",
			"cross_version_login/R117-15572.63.0_reven-vmtest_20231031_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR118",
		Desc: "Loads the data of milestone R118 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R118-15604.60.0_reven-vmtest_20231123",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R118-15604.60.0_reven-vmtest_20231123_config.json",
			"cross_version_login/R118-15604.60.0_reven-vmtest_20231123_data.tar.gz",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "crossVersionTpmDynamicR119",
		Desc: "Loads the data of milestone R119 from the tpm dynamic device",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		SetUpTimeout:    crossVersionSetUpTimeout,
		ResetTimeout:    crossVersionResetTimeout,
		TearDownTimeout: crossVersionTearDownTimeout,
		Parent:          "crossVersionBackup",
		Impl: &crossVersionFixtImpl{
			dataPrefix: "R119-15633.69.0_reven-vmtest_20231212",
			useCurrent: false,
		},
		Data: []string{
			"cross_version_login/R119-15633.69.0_reven-vmtest_20231212_config.json",
			"cross_version_login/R119-15633.69.0_reven-vmtest_20231212_data.tar.gz",
		},
	})
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
	dataPrefix     string
	useCurrent     bool
	dataPath       string
	webauthnServer *u2fd.WebAuthnHTTPServer
}

// CrossVersionLoginFixture contains the config list for the login data used in cross version testing.
type CrossVersionLoginFixture struct {
	ConfigList  []util.CrossVersionLoginConfig
	WebAuthnURL string
}

// SetUp loads the data of the milestone specified in the crossVersionFixtImpl.dataPrefix
func (f *crossVersionFixtImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	parentData := s.ParentValue().(backupFixture)

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

	if f.useCurrent {
		dataPath = filepath.Join(tmpDir, "data.tar.gz")
		configPath = filepath.Join(tmpDir, "config.json")
		s.Log("Preparing login data of current version")
		if err := util.PrepareCrossVersionLoginData(ctx, s.Logf, helper.CmdHelper, dataPath, configPath, parentData.WebAuthnURL); err != nil {
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
