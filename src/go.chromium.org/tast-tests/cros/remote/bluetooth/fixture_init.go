// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"time"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/testing"
)

const (
	// BluetoothCooldownTimeout is the max amount of time that Bluetooth allows
	// for all combined cooldown items. This includes:
	// 5 minutes max amount of time needed for device temperature to cooldown
	// 2 minutes max amount of time needed for cpu idle state activity to drop
	// 3 minutes max amount of time needed for cpu pkg state activity to drop
	BluetoothCooldownTimeout = 10 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWithBluetoothEnabledFlossDisabled",
		Desc: "Logs into a user session and enables Bluetooth during set up and disables it during tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    false,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWithBluetoothEnabledFlossEnabled",
		Desc: "Logs into a user session and enables Bluetooth during set up and disables it during tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    true,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith1BTPeerFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    1,
			FlossEnabled:   false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith1BTPeerPowerFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth Floss stack with power measurements, and connects to 1 btpeer",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    1,
			FlossEnabled:   true,
			PowerEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith1BTPeerPowerFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth BlueZ stack with power measurements, and connects to 1 btpeer",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    1,
			FlossEnabled:   false,
			PowerEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith2BTPeersPowerFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth, BlueZ stack with power measurements, and connects to 2 btpeer",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    2,
			FlossEnabled:   false,
			PowerEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith2BTPeersPowerFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth, Floss stack with power measurements, and connects to 2 btpeer",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    2,
			FlossEnabled:   true,
			PowerEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith1BTPeerFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    1,
			FlossEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith2BTPeersFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 2 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    2,
			FlossEnabled:   false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith2BTPeersFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 2 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    2,
			FlossEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith3BTPeersFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 3 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    3,
			FlossEnabled:   false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 3*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith3BTPeersFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 3 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    3,
			FlossEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 3*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith4BTPeersFlossDisabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 4 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    4,
			FlossEnabled:   false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 4*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeUIDisabledWith4BTPeersFlossEnabled",
		Desc: "Disables Chrome UI, enables Bluetooth, and connects to 4 btpeers",
		Contacts: []string{
			"jaredbennett@google.com",
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: false,
			BTPeerCount:    4,
			FlossEnabled:   true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 4*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith1BTPeerFlossDisabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     1,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith1BTPeerFlossEnabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     1,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith2BTPeersFlossDisabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 2 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     2,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith2BTPeersFlossEnabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 2 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     2,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith3BTPeersFlossDisabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 3 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     3,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 3*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith3BTPeersFlossEnabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 3 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     3,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 3*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith4BTPeersFlossDisabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 4 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     4,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 4*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith4BTPeersFlossEnabled",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 4 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:  true,
			BTPeerCount:     4,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			FlossEnabled:    true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
		},
		SetUpTimeout:    setUpTimeout + 4*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeOobeWith1BTPeerFlossDisabled",
		Desc: "Puts the DUT into OOBE, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:        true,
			BTPeerCount:           1,
			EnableFeatures:        []string{chromeFeatureOobeHidDetectionRevamp},
			DisableFeatures:       []string{},
			LoginMode:             ui.LoginMode_LOGIN_MODE_NO_LOGIN,
			EnableHidScreenOnOobe: true,
			FlossEnabled:          false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarSigninKey,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeOobeWith1BTPeerFlossEnabled",
		Desc: "Puts the DUT into OOBE, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI:        true,
			BTPeerCount:           1,
			EnableFeatures:        []string{chromeFeatureOobeHidDetectionRevamp},
			DisableFeatures:       []string{},
			LoginMode:             ui.LoginMode_LOGIN_MODE_NO_LOGIN,
			EnableHidScreenOnOobe: true,
			FlossEnabled:          true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarSigninKey,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInAsUserWithFastPairAnd1BTPeerFlossDisabled",
		Desc: "Logs into a chrome as a specific user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"jaredbennett@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			FlossEnabled:           false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInAsUserWithFastPairAnd1BTPeerPowerFlossDisabled",
		Desc: "Logs into a chrome as a specific user and enables Bluetooth, FastPair, BlueZ and connects to 1 btpeer. Monitor and record power",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			FlossEnabled:           false,
			PowerEnabled:           true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInAsUserWithFastPairAnd1BTPeerPowerFlossEnabled",
		Desc: "Logs into a chrome as a specific user and enables Bluetooth, FastPair, Floss and connects to 1 btpeer. Monitor and record power",
		Contacts: []string{
			"jiangzp@google.com",
			"chromeos-bt-team@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			FlossEnabled:           true,
			PowerEnabled:           true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer + BluetoothCooldownTimeout,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
			serviceDepMetricsService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInAsUserWithFastPairAnd1BTPeerFlossEnabled",
		Desc: "Logs into a chrome as a specific user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"jaredbennett@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			FlossEnabled:           true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossDisabled",
		Desc: "Logs into two Chromebooks as the same user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"dclasson@google.com",
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			UseSameGaiaLogin:       true,
			RequireCompanionDUT:    true,
			FlossEnabled:           false,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    2*setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    2*resetTimeout + btpeerResetBuffer,
		TearDownTimeout: 2*tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: 2 * postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "twoChromebooksLoggedInWithFastPairAnd1BTPeerFlossEnabled",
		Desc: "Logs into two Chromebooks as the same user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"dclasson@google.com",
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableChromeUI: true,
			BTPeerCount:    1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              ui.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			UseSameGaiaLogin:       true,
			RequireCompanionDUT:    true,
			FlossEnabled:           true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    2*setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    2*resetTimeout + btpeerResetBuffer,
		TearDownTimeout: 2*tearDownTimeout + btpeerResetBuffer,
		PostTestTimeout: 2 * postTestTimeout,
		ServiceDeps: []string{
			serviceDepBluetoothUIService,
			serviceDepBluetoothService,
			serviceDepChromeService,
			serviceDepUpstartService,
		},
	})
}
