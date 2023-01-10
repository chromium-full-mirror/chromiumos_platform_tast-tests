// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	btc "chromiumos/tast/common/bluetooth"
	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/common/tape"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/dbus"
	"chromiumos/tast/remote/wificell/fileutil"
	"chromiumos/tast/rpc"
	bts "chromiumos/tast/services/cros/bluetooth"
	chromeService "chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

// Fixture variable keys.
const (
	// fixtureVarBTPeers is the name of the tast var that specifies a
	// comma-separated list of btpeer host addresses.
	//
	// This is an optional override to the usual btpeer addresses which are normally
	// resolved based on the DUT hostname.
	fixtureVarBTPeers = "btpeers"

	fixtureVarSigninKey = "ui.signinProfileTestExtensionManifestKey"

	// If cleanup is not called for the OTAs leased by Tape for the duration of the
	// fixture, the accounts will be released after |fixtureVarFastPairTapeCleanupTimeout|
	// expires. Thus the timeout should be longer than the duration of the fixture setup and
	// all tests that it will run.
	// TODO(b/264412597): This timeout must be kept in sync with the total timeouts of all tests that
	// can run on the fixtures that use Tast.
	fixtureVarFastPairTapeCleanupTimeout = 15 * time.Minute

	// These variables can be overridden by specifying a custom value in the command
	// line, e.g. "--vars=bluetooth.FastPairChromeUsername=XXXX", which can be used
	// for local testing. Otherwise uses the default value for GAIA login.
	fixtureVarFastPairChromeUsername = "bluetooth.FastPairChromeUsername"
	fixtureVarFastPairChromePassword = "bluetooth.FastPairChromePassword"
)

// Public test variable keys that are used in multiple tests
const (
	// TestVarFastPairAntispoofingKeyPem is used for setting the antispoofing key, which is
	// required for Initial Pairing Fast Pair V2 devices. The variable can be overridden by
	// specifying a custom value in the command line,
	// "--vars=bluetooth.FastPairAntispoofingKeyPem=XXXX", which can be used
	// for local testing. Otherwise uses the default value.
	TestVarFastPairAntispoofingKeyPem = "bluetooth.FastPairAntispoofingKeyPem"

	// TestVarFastPairAccountKey is used for setting the account key, which is
	// required for Subsequent Pairing Fast Pair V2 devices. The variable can be overridden
	// by specifying a custom value in the command line,
	// "--vars=bluetooth.FastPairAccountKey=XXXX", which can be used
	// for local testing. Otherwise uses the default value.
	TestVarFastPairAccountKey = "bluetooth.FastPairAccountKey"
)

// Used for Fake login for the Bluetooth UI tests.
const (
	defaultChromeUsername = "testuser@gmail.com"
	defaultChromePassword = "testpass"
)

const (
	serviceDepBTTestService = "tast.cros.bluetooth.BTTestService"
	serviceDepChromeService = "tast.cros.browser.ChromeService"
)

const (
	dbusServiceBluetoothBluez = "org.bluez"
	dbusServiceBluetoothFloss = "org.chromium.bluetooth"
)

// Chrome features.
const (
	chromeFeatureOobeHidDetectionRevamp = "OobeHidDetectionRevamp"
	chromeFeatureFastPair               = "FastPair"
	chromeFeatureFastPairSavedDevices   = "FastPairSavedDevices"
)

const (
	setUpTimeout    = 80 * time.Second
	resetTimeout    = 65 * time.Second
	tearDownTimeout = 70 * time.Second
	postTestTimeout = 1 * time.Second

	// btpeerTimeoutBuffer is added to fixture per btpeer expected to give
	// additional time to manage each btpeer.
	btpeerTimeoutBuffer = 15 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWithBluetoothEnabled",
		Desc: "Logs into a user session and enables Bluetooth during set up and disables it during tear down",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       chromeService.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		}),
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith1BTPeer",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount:     1,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       chromeService.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		}),
		Vars:            []string{fixtureVarBTPeers},
		SetUpTimeout:    setUpTimeout + btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith2BTPeers",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 2 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount:     2,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       chromeService.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		}),
		Vars:            []string{fixtureVarBTPeers},
		SetUpTimeout:    setUpTimeout + 2*btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith3BTPeers",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 3 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount:     3,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       chromeService.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		}),
		Vars:            []string{fixtureVarBTPeers},
		SetUpTimeout:    setUpTimeout + 3*btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInWith4BTPeers",
		Desc: "Logs into a user session, enables Bluetooth, and connects to 4 btpeers",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount:     4,
			EnableFeatures:  []string{},
			DisableFeatures: []string{},
			LoginMode:       chromeService.LoginMode_LOGIN_MODE_FAKE_LOGIN,
		}),
		Vars:            []string{fixtureVarBTPeers},
		SetUpTimeout:    setUpTimeout + 4*btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeOobeWith1BTPeer",
		Desc: "Puts the DUT into OOBE, enables Bluetooth, and connects to 1 btpeer",
		Contacts: []string{
			"chadduffin@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount:           1,
			EnableFeatures:        []string{chromeFeatureOobeHidDetectionRevamp},
			DisableFeatures:       []string{},
			LoginMode:             chromeService.LoginMode_LOGIN_MODE_NO_LOGIN,
			EnableHidScreenOnOobe: true,
		}),
		Vars:            []string{fixtureVarBTPeers, fixtureVarSigninKey},
		SetUpTimeout:    setUpTimeout + btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "chromeLoggedInAsUserWithFastPairAnd1BTPeer",
		Desc: "Logs into a chrome as a specific user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"jaredbennett@chromium.org",
			"cros-connectivity@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount: 1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              chromeService.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    setUpTimeout + btpeerTimeoutBuffer,
		ResetTimeout:    resetTimeout + btpeerTimeoutBuffer,
		TearDownTimeout: tearDownTimeout + btpeerTimeoutBuffer,
		PostTestTimeout: postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "twoChromebooksLoggedInWithFastPairAnd1BTPeer",
		Desc: "Logs into two Chromebooks as the same user and enables Bluetooth, FastPair, and connects to 1 btpeer",
		Contacts: []string{
			"dclasson@google.com",
			"chromeos-sw-engprod@google.com",
			"chromeos-cross-device-eng@google.com",
		},
		Impl: newFixture(&fixtureFeatures{
			BTPeerCount: 1,
			EnableFeatures: []string{
				chromeFeatureFastPair,
				chromeFeatureFastPairSavedDevices,
			},
			DisableFeatures:        []string{},
			LoginMode:              chromeService.LoginMode_LOGIN_MODE_GAIA_LOGIN,
			UseFastPairTapeAccount: true,
			RequireCompanionDUT:    true,
		}),
		Vars: []string{
			fixtureVarBTPeers,
			fixtureVarFastPairChromeUsername,
			fixtureVarFastPairChromePassword,
			tape.ServiceAccountVar,
		},
		SetUpTimeout:    2*setUpTimeout + btpeerTimeoutBuffer,
		ResetTimeout:    2*resetTimeout + btpeerTimeoutBuffer,
		TearDownTimeout: 2*tearDownTimeout + btpeerTimeoutBuffer,
		PostTestTimeout: 2 * postTestTimeout,
		ServiceDeps:     []string{serviceDepBTTestService, serviceDepChromeService},
	})
}

type fixtureFeatures struct {
	// BTPeerCount requires the specified amount of btpeers to exist in the
	// testbed and connects to them during setup. A testbed can have more btpeers
	// than the BTPeerCount, but only that many connections are configured.
	BTPeerCount int

	// EnableFeatures is the list of features that will be enabled when starting Chrome.
	EnableFeatures []string

	// DisableFeatures is the list of features that will be enabled when starting Chrome.
	DisableFeatures []string

	// LoginMode is what the resulting login mode should be after starting Chrome.
	LoginMode chromeService.LoginMode

	// EnableHidScreenOnOobe enables HID detection screen when in OOBE.
	EnableHidScreenOnOobe bool

	// UseFastPairTapeAccount uses an OTA to login, selected by TAPE from the Fast
	// Pair OTA pool.
	UseFastPairTapeAccount bool

	// RequireFastPairUserVars enables retrieving chrome user credentials from
	// fixture vars, and requires that they are provided. Required for all Fast
	// Pair tests that use a GAIA login.
	RequireFastPairUserVars bool

	// RequireCompanionDUT enables logging in on a second Chromebook with the same
	// user account, which is necessary for Fast Pair Multi-DUT tests. Enforces that
	// a companion DUT is connected and passed in.
	RequireCompanionDUT bool
}

// DUTConfig groups DUT-specific fixture configs and utils.
type DUTConfig struct {
	// DUT is the connection to the primary DUT.
	DUT *dut.DUT

	// DUTRPCClient is a gRPC client that remains connected to the DUT throughout
	// the life of the test fixture. This can be used to create clients to
	// additional local tast services.
	DUTRPCClient *rpc.Client

	// BTS is a client of the BTTestService that is used to interact with and
	// manage bluetooth on the DUT.
	BTS bts.BTTestServiceClient

	// ChromeService is a client of the ChromeService that is used to start Chrome.
	ChromeService chromeService.ChromeServiceClient
}

func newDUTConfig(ctx context.Context, dut *dut.DUT, RPCHint *testing.RPCHint) (*DUTConfig, error) {
	rpcClient, err := rpc.Dial(ctx, dut, RPCHint)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to the local gRPC service on DUT %s", dut.HostName())
	}
	return &DUTConfig{
		DUT:           dut,
		DUTRPCClient:  rpcClient,
		BTS:           bts.NewBTTestServiceClient(rpcClient.Conn),
		ChromeService: chromeService.NewChromeServiceClient(rpcClient.Conn),
	}, nil
}

// FixtValue is the value of the test fixture accessible within a test. All
// variables are configured in fixture.SetUp so that tests can use them without
// any further configuration.
type FixtValue struct {
	// BTPeers is a list of chameleond clients that are connected to each btpeer
	// available to the test fixture.
	BTPeers []chameleon.Chameleond

	// tapeAccountManager stores the OTA Manager returned by tape.NewClient.
	tapeAccountManager *tape.OwnedTestAccountManager

	// tapeAccount stores the OTA returned by tape.NewClient.
	tapeAccount *tape.OwnedTestAccount

	// DUT is the connection to the primary DUT.
	DUT *dut.DUT

	// DUTRPCClient is a gRPC client that remains connected to the DUT throughout
	// the life of the test fixture. This can be used to create clients to
	// additional local tast services.
	DUTRPCClient *rpc.Client

	// BTS is a client of the BTTestService that is used to interact with and
	// manage bluetooth on the primary DUT.
	BTS bts.BTTestServiceClient

	// ChromeService is a client of the ChromeService that is used to start Chrome.
	ChromeService chromeService.ChromeServiceClient

	// DUTs stores the dut-specific configurations for each DUT in the fixture.
	// The first item in this list refers to the primary DUT, and subsequent items
	// refer to companion DUTs.
	DUTConfigs []*DUTConfig
}

// PrimaryDUTConfig returns the DUTConfig for the primary DUT.
func (fv *FixtValue) PrimaryDUTConfig() *DUTConfig {
	return fv.DUTConfigs[0]
}

// CompanionDUTConfig returns the DUTConfig for the specified companion DUT. The
// companion DUTs are numbered starting from 1. Will panic if the companion DUT
// has not been configured.
func (fv *FixtValue) CompanionDUTConfig(companionNum uint) *DUTConfig {
	if len(fv.DUTConfigs) <= 1 {
		panic("fixture not configured to use companion DUTs")
	}
	if companionNum == 0 {
		panic("companionNums start at 1 for the first companion DUT")
	}
	return fv.DUTConfigs[companionNum]
}

type fixture struct {
	features                      *fixtureFeatures
	fv                            *FixtValue
	bluetoothServicesDBusMonitors []*dbus.Monitor
	fastPairEnabled               bool
}

func newFixture(features *fixtureFeatures) *fixture {
	return &fixture{
		features: features,
		fv:       &FixtValue{},
	}
}

// SetUp preforms fixture setup actions. All fixtureFeatures are configured.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Parse OOBE fixture var.
	var signinProfileTestExtensionID string
	if tf.features.EnableHidScreenOnOobe {
		var ok bool
		signinProfileTestExtensionID, ok = s.Var(fixtureVarSigninKey)
		if !ok {
			s.Fatal("Failed to get sign-in key variable required for OOBE tests")
		}
	}

	// Determine if fast pair is an enabled feature for later reference.
	tf.fastPairEnabled = false
	for _, feature := range tf.features.EnableFeatures {
		if feature == chromeFeatureFastPairSavedDevices {
			tf.fastPairEnabled = true
			break
		}
	}

	// Resolve chrome user credentials.
	s.Log("Resolving chrome user credentials")
	var chromeUsername, chromePassword string
	if tf.features.RequireFastPairUserVars {
		// Fast Pair tests require GAIA credentials to be provided, which can be
		// passed via CLI or will use the default credentials.
		chromeUsername = s.RequiredVar(fixtureVarFastPairChromeUsername)
		chromePassword = s.RequiredVar(fixtureVarFastPairChromePassword)
		s.Logf("Using Fast Pair test chrome user credentials for user %q", chromeUsername)
	} else if tf.features.UseFastPairTapeAccount {
		// Create a tape account manager and lease a test account for the duration
		// of the fixture.
		s.Log("Leasing Fast Pair OTA chrome user with Tape")
		tapeServiceAccountVar := s.RequiredVar(tape.ServiceAccountVar)
		var err error
		tf.fv.tapeAccountManager, tf.fv.tapeAccount, err = tape.NewOwnedTestAccountManager(
			ctx,
			[]byte(tapeServiceAccountVar),
			true,
			tape.WithTimeout(int32(fixtureVarFastPairTapeCleanupTimeout.Seconds())),
			tape.WithPoolID(tape.CrossDeviceFastPair),
		)
		if err != nil {
			s.Fatal("Failed to create a tape account manager and lease an account: ", err)
		}
		chromeUsername = tf.fv.tapeAccount.Username
		chromePassword = tf.fv.tapeAccount.Password
		s.Logf("Using Fast Pair OTA chrome user credentials leased with Tape for user %q", chromeUsername)
	} else {
		// By default, use the default username/password used for Fake login.
		chromeUsername = defaultChromeUsername
		chromePassword = defaultChromePassword
		s.Log("Using default fake chrome user credentials")
	}

	// Connect to btpeers and reset them to a fresh state.
	if err := tf.setUpBTPeers(ctx, s, tf.features.BTPeerCount); err != nil {
		s.Fatal("Failed to set up btpeers: ", err)
	}
	if err := tf.resetBTPeers(ctx); err != nil {
		s.Fatal("Failed to reset all btpeers: ", err)
	}

	// Configure primary DUT.
	primaryDUTConfig, err := newDUTConfig(s.FixtContext(), s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to configure primary DUT: ", err)
	}
	tf.fv.DUTConfigs = []*DUTConfig{primaryDUTConfig}

	// Save shortcuts for primary DUT configs for ease of use in most tests.
	tf.fv.DUTRPCClient = primaryDUTConfig.DUTRPCClient
	tf.fv.BTS = primaryDUTConfig.BTS
	tf.fv.ChromeService = primaryDUTConfig.ChromeService

	// Configure companion DUT.
	if tf.features.RequireCompanionDUT {
		companionDUT := s.CompanionDUT("cd1")
		if companionDUT == nil {
			s.Fatal("Failed to get companion DUT cd1")
		}
		companionDUTConfig, err := newDUTConfig(s.FixtContext(), companionDUT, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to configure companion DUT: ", err)
		}
		tf.fv.DUTConfigs = append(tf.fv.DUTConfigs, companionDUTConfig)
	}

	// Perform per-DUT setup actions.
	for _, dutConfig := range tf.fv.DUTConfigs {
		dutName := dutConfig.DUT.HostName()
		s.Logf("SetUp for DUT %s started", dutName)

		// Start capturing incoming and outgoing bluez and floss D-Bus messages.
		bluetoothServicesDBusMonitor, err := dbus.StartMonitor(
			ctx,
			dutConfig.DUT.Conn(),
			"--system",
			fmt.Sprintf("sender='%s'", dbusServiceBluetoothBluez),
			fmt.Sprintf("sender='%s'", dbusServiceBluetoothFloss),
			fmt.Sprintf("destination='%s'", dbusServiceBluetoothBluez),
			fmt.Sprintf("destination='%s'", dbusServiceBluetoothFloss),
		)
		if err != nil {
			s.Fatal("Failed to start dbus-monitor listening to bluez and floss service messages: ", err)
		}
		tf.bluetoothServicesDBusMonitors = append(tf.bluetoothServicesDBusMonitors, bluetoothServicesDBusMonitor)

		// Start Chrome with the features and login mode provided by the test fixture.
		if _, err := dutConfig.ChromeService.New(ctx, &chromeService.NewRequest{
			LoginMode:       tf.features.LoginMode,
			EnableFeatures:  tf.features.EnableFeatures,
			DisableFeatures: tf.features.DisableFeatures,
			Credentials: &chromeService.NewRequest_Credentials{
				Username: chromeUsername,
				Password: chromePassword,
			},
			EnableHidScreenOnOobe:        tf.features.EnableHidScreenOnOobe,
			SigninProfileTestExtensionId: signinProfileTestExtensionID,
		}); err != nil {
			s.Fatalf("Failed to log into chrome on DUT %s: %v", dutName, err)
		}

		// Reset and enable bluetooth.
		if _, err := dutConfig.BTS.DisableBluetoothAdapter(ctx, &emptypb.Empty{}); err != nil {
			s.Fatalf("Failed to disable bluetooth adapter on DUT %s: %v", dutName, err)
		}
		if _, err := dutConfig.BTS.EnableBluetoothAdapter(ctx, &emptypb.Empty{}); err != nil {
			s.Fatalf("Failed to enable bluetooth adapter on DUT %s: %v", dutName, err)
		}
		if err := tf.resetDutBluetoothState(ctx, dutConfig); err != nil {
			s.Errorf("Failed to reset state of DUT %s: %v", dutName, err)
		}

		s.Logf("SetUp for DUT %s completed", dutName)
	}

	// Save collected bluez D-Bus messages collected thus far.
	if err := tf.logAllDBusMonitorBluetoothMessages(ctx, "SetUp"); err != nil {
		s.Fatal("Failed to collect dbus-monitor bluez logs: ", err)
	}

	return tf.fv
}

// Reset is called by the framework after each test (except for the last one) to
// do a light-weight reset of the environment to the original state.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) Reset(ctx context.Context) error {
	if err := tf.resetBTPeers(ctx); err != nil {
		return errors.Wrap(err, "failed to reset all btpeers")
	}
	for _, dutConfig := range tf.fv.DUTConfigs {
		if err := tf.resetDutBluetoothState(ctx, dutConfig); err != nil {
			return errors.Wrapf(err, "failed to reset state of DUT %s", dutConfig.DUT.HostName())
		}
		if _, err := dutConfig.BTS.DisableBluetoothAdapter(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to disable bluetooth adapter on DUT %s", dutConfig.DUT.HostName())
		}
		if _, err := dutConfig.BTS.EnableBluetoothAdapter(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to enable bluetooth adapter on DUT %s", dutConfig.DUT.HostName())
		}
	}
	if err := tf.logAllDBusMonitorBluetoothMessages(ctx, "Reset"); err != nil {
		return errors.Wrap(err, "failed to collect dbus-monitor bluetooth logs")
	}
	return nil
}

// PreTest is called by the framework before each test to do a light-weight set
// up for the test.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

// PostTest is called by the framework after each test to tear down changes
// PreTest made.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Save any new dbus logs that occurred during the test.
	if err := tf.logAllDBusMonitorBluetoothMessages(ctx, "PostTest"); err != nil {
		s.Fatal("Failed to collect dbus-monitor bluez logs: ", err)
	}
}

// TearDown is called by the framework to tear down the environment SetUp set
// up.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Reset btpeers to original state.
	if err := tf.resetBTPeers(ctx); err != nil {
		s.Error("Failed to reset all btpeers: ", err)
	}

	// Tear down each DUT.
	for _, dutConfig := range tf.fv.DUTConfigs {
		dutName := dutConfig.DUT.HostName()
		s.Logf("TearDown for DUT %s started", dutName)

		// Clean up bluetooth state and disable adapter.
		if err := tf.resetDutBluetoothState(ctx, dutConfig); err != nil {
			s.Errorf("Failed to reset state of DUT %s: %v", dutName, err)
		}
		s.Logf("Disabling bluetooth on DUT %s", dutName)
		if _, err := dutConfig.BTS.DisableBluetoothAdapter(ctx, &emptypb.Empty{}); err != nil {
			s.Errorf("Failed to disable bluetooth adapter on DUT %s: %v", dutName, err)
		}

		// Clean up chrome login state.
		if _, err := dutConfig.ChromeService.Close(ctx, &emptypb.Empty{}); err != nil {
			s.Error("Failed to close Chrome on the DUT: ", err)
		}

		// Close gRPC connection to DUT.
		if err := dutConfig.DUTRPCClient.Close(ctx); err != nil {
			s.Error("Failed to close gRPC connection to DUT: ", err)
		}

		s.Logf("TearDown for DUT %s completed", dutName)
	}

	// Stop dbus monitors.
	if err := tf.logAllDBusMonitorBluetoothMessages(ctx, "TearDown"); err != nil {
		s.Error("Failed to collect dbus-monitor bluez logs: ", err)
	}
	for _, dbusMonitor := range tf.bluetoothServicesDBusMonitors {
		if err := dbusMonitor.Close(); err != nil {
			s.Error("Failed to close dbus-monitor: ", err)
		}
	}

	// Clean up Tape helpers if it was used for credentials.
	if tf.features.UseFastPairTapeAccount {
		s.Log("Cleaning up Fast Pair OTA provisioned with Tape: ", tf.fv.tapeAccount.Username)
		if err := tf.fv.tapeAccountManager.CleanUp(ctx); err != nil {
			s.Error("Failed to clean up Tape OTA: ", err)
		}
	}
}

func (tf *fixture) setUpBTPeers(ctx context.Context, s *testing.FixtState, requiredBTPeers int) error {
	ctx, st := timing.Start(ctx, fmt.Sprintf("setUpBTPeers_%d", requiredBTPeers))
	defer st.End()
	if requiredBTPeers <= 0 {
		return nil
	}
	var btpeerAddresses []string
	if btpeersVar, isSet := s.Var(fixtureVarBTPeers); isSet && btpeersVar != "" {
		btpeerAddresses = strings.Split(btpeersVar, ",")
		if len(btpeerAddresses) < requiredBTPeers {
			return errors.Errorf("fixture requires at least %d btpeers, but "+
				"only %d were provided in the %s tast var (%q)",
				requiredBTPeers, len(btpeerAddresses),
				fixtureVarBTPeers, btpeersVar)
		}
		btpeerAddresses = btpeerAddresses[:requiredBTPeers]
	} else {
		// Imply btpeer hostnames based on DUT hostname.
		btpeerAddresses = make([]string, requiredBTPeers)
		dutHostname := strings.Split(s.DUT().HostName(), ":")[0]
		if dutHostname == "localhost" || dutHostname == "" || dutHostname == "127.0.0.1" {
			for i := 0; i < requiredBTPeers; i++ {
				btpeerAddresses[i] = fmt.Sprintf("localhost:%d", 2201+i)
			}
			exampleTastCall := fmt.Sprintf("tast run --var=%s=%s %s <test>",
				fixtureVarBTPeers, strings.Join(btpeerAddresses, ","),
				s.DUT().HostName())
			return errors.Errorf("btpeer hostname resolution not supported "+
				"when DUT hostname is %q. If tast is being run in a local "+
				"development environment outside of the lab, ssh tunnel to the "+
				"btpeers outside of the chroot and provide the local forwarded"+
				" address to tast using the %q tast var (e.g. %q)",
				dutHostname, fixtureVarBTPeers, exampleTastCall)
		}
		for i := 0; i < requiredBTPeers; i++ {
			btpeerAddresses[i] = fmt.Sprintf("%s-btpeer%d", dutHostname, i+1)
		}
	}
	testing.ContextLogf(ctx, "Connecting to %d btpeers: %s",
		len(btpeerAddresses), strings.Join(btpeerAddresses, ", "))
	btpeers, err := btc.ConnectToBTPeers(ctx, btpeerAddresses)
	if err != nil {
		return err
	}
	tf.fv.BTPeers = btpeers
	testing.ContextLogf(ctx, "Successfully connected to %d btpeers",
		len(tf.fv.BTPeers))
	return nil
}

// resetBTPeers resets each configured btpeer to return them to their normal
// state and clear any changes a test may have made to them.
// Each btpeer is reset in parallel to save time. If any reset fails, the first
// error is returned and any pending resets are cancelled.
func (tf *fixture) resetBTPeers(ctx context.Context) error {
	ctx, st := timing.Start(ctx, fmt.Sprintf("resetBTPeers_%d", len(tf.fv.BTPeers)))
	defer st.End()
	if len(tf.fv.BTPeers) == 0 {
		return nil
	}
	testing.ContextLogf(ctx, "Resetting %d btpeers", len(tf.fv.BTPeers))
	resetCtx, cancelResetCtx := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelResetCtx()
	resetGroup, resetCtx := errgroup.WithContext(resetCtx)
	for i, btpeer := range tf.fv.BTPeers {
		// Note: loop var values are copied to inner vars for use in func literal.
		i := i
		btpeer := btpeer
		resetGroup.Go(func() error {
			return tf.resetBTPeer(resetCtx, i, btpeer)
		})
	}
	if err := resetGroup.Wait(); err != nil {
		return errors.Wrap(err, "failed to reset btpeers")
	}
	return nil
}

func (tf *fixture) resetBTPeer(ctx context.Context, btpeerIndex int, btpeer chameleon.Chameleond) error {
	// Reset the base chameleond service state.
	if err := btpeer.Reset(ctx); err != nil {
		return errors.Wrapf(err, "failed to reset chameleond on btpeer[%d] at %q", btpeerIndex, btpeer.Host())
	}
	// Reset the bluetooth service state, through the keyboard device interface
	// since this method is not exposed at a higher level.
	if err := btpeer.BluetoothKeyboardDevice().ResetStack(ctx, ""); err != nil {
		return errors.Wrapf(err, "failed to reset bluetooth stack on btpeer[%d] at %q", btpeerIndex, btpeer.Host())
	}
	return nil
}

func (tf *fixture) logAllDBusMonitorBluetoothMessages(ctx context.Context, logName string) error {
	for i, dbusMonitor := range tf.bluetoothServicesDBusMonitors {
		if err := tf.logDBusMonitorBluetoothMessages(ctx, fmt.Sprintf("dut%d", i), logName, dbusMonitor); err != nil {
			return errors.Wrap(err, "failed to collect dbus-monitor logs")
		}
	}
	return nil
}

func (tf *fixture) logDBusMonitorBluetoothMessages(ctx context.Context, dutFolderName, logName string, monitor *dbus.Monitor) error {
	ctx, st := timing.Start(ctx, "logDBusMonitorBluetoothMessages")
	defer st.End()
	// Prepare output file, which looks like "dbus_monitor_bluetooth/dutFolderName/dstLogFilename"
	dstLogFilename := tf.buildLogFilename(logName)
	dstFilePath := filepath.Join("dbus_monitor_bluetooth", dutFolderName, dstLogFilename)
	f, err := fileutil.PrepareOutDirFile(ctx, dstFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to prepare output dir file %q", dstFilePath)
	}
	// Dump buffer of collected logs to file, for the passed |monitor|.
	if err := monitor.Dump(f); err != nil {
		return errors.Wrapf(err, "failed to dump dbus-monitor logs to %q", dstFilePath)
	}
	return nil
}

// buildLogFilename builds a log filename with a minimal timestamp prefix, all
// the name parts in the middle delimited by "_" with non-word characters
// replaced with underscores, and a ".log" file extension.
//
// This not only communicates the time of the log to users, but keeps similar
// files in chronological order within the same directory when displayed sorted
// by name (alphanumerical order) by most programs.
//
// Example result: "20220523-122753_dbus_bluetooth_PostTest"
func (tf *fixture) buildLogFilename(nameParts ...string) string {
	// Build timestamp prefix.
	timestamp := time.Now().Format("20060102-150405")
	// Join and sanitize name parts.
	name := strings.Join(nameParts, "_")
	name = regexp.MustCompile("\\W").ReplaceAllString(name, "_")
	name = regexp.MustCompile("_+").ReplaceAllString(name, "_")
	// Combine timestamp, name, and extension.
	return fmt.Sprintf("%s_%s.log", timestamp, name)
}

// resetDutBluetoothState resets the bluetooth state of the DUT so that it is
// ready for tests.
func (tf *fixture) resetDutBluetoothState(ctx context.Context, dutConfig *DUTConfig) error {
	dutName := dutConfig.DUT.HostName()
	// Handle Fast Pair UI reset needs.
	if tf.fastPairEnabled {
		testing.ContextLogf(ctx, "Removing all saved bluetooth devices via UI on DUT %s", dutName)
		if _, err := dutConfig.BTS.RemoveAllSavedDevices(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to remove all saved bluetooth devices via UI on DUT %s", dutName)
		}
		testing.ContextLogf(ctx, "Closing all UI notifications on DUT %s", dutName)
		if _, err := dutConfig.BTS.CloseNotifications(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to close all UI notifications on DUT %s", dutName)
		}
	}

	// Reset the state of the bluetooth adapter.
	if _, err := dutConfig.BTS.DisconnectAllDevices(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to disconnected all bluetooth devices")
	}
	if _, err := dutConfig.BTS.RemoveAllDevices(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to remove all bluetooth devices")
	}
	return nil
}
