// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/common/tape"
	"chromiumos/tast/common/utils"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/log"
	"chromiumos/tast/rpc"
	bts "chromiumos/tast/services/cros/bluetooth"
	chromeService "chromiumos/tast/services/cros/ui"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/timing"
)

// Fixture variable keys.
const (
	// fixtureVarBTPeers is the name of the tast var that specifies a
	// comma-separated list of btpeer host addresses.
	//
	// This is an optional override to the usual btpeer hosts which are normally
	// resolved based on the DUT hostname.
	fixtureVarBTPeers = "bluetooth.BTPeers"

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

// Non-const Fixture variable keys.
var (
	fixtureVarFastPairExtraArgs = []string{"--enable-logging", `--vmodule=*blue*=3,ble_*=3,fast_pair*=3,
		quick_pair*=3,pairer_broker*=3,device_address_map*=3,retroactive_pairing*=3,device_image_store*=3,
		companion_app*=3,saved_device*=3,device_metadata*=3,footprints_fetcher*=3,scanner_broker*=3,
		oauth_http_fetcher*=3,message_stream*=3,unauthenticated_http_fetcher*=3,battery_update_message_handler*=3`,
		"--log=level=1"}
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

	// btpeerSetUpBuffer is added to the setup phase per btpeer expected to give
	// additional time to set up each btpeer.
	btpeerSetUpBuffer = 90 * time.Second

	// btpeerResetBuffer is added to fixture per btpeer expected to give
	// additional time to reset each btpeer.
	btpeerResetBuffer = 15 * time.Second
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
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
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
		SetUpTimeout:    setUpTimeout + 2*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 2*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 2*btpeerResetBuffer,
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
		SetUpTimeout:    setUpTimeout + 3*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 3*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 3*btpeerResetBuffer,
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
		SetUpTimeout:    setUpTimeout + 4*btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + 4*btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + 4*btpeerResetBuffer,
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
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
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
		SetUpTimeout:    setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    resetTimeout + btpeerResetBuffer,
		TearDownTimeout: tearDownTimeout + btpeerResetBuffer,
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
		SetUpTimeout:    2*setUpTimeout + btpeerSetUpBuffer,
		ResetTimeout:    2*resetTimeout + btpeerResetBuffer,
		TearDownTimeout: 2*tearDownTimeout + btpeerResetBuffer,
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

type bTPeerCompanion struct {
	host                    string
	sshConn                 *ssh.Conn
	chameleondClient        chameleon.Chameleond
	chameleondPortForwarder *ssh.Forwarder
	logCollector            log.Collector
}

// FixtValue is the value of the test fixture accessible within a test. All
// variables are configured in fixture.SetUp so that tests can use them without
// any further configuration.
type FixtValue struct {
	bTPeerCompanions []*bTPeerCompanion

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
	bluetoothServicesDBusMonitors []*log.DBusMonitorCollector
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
		bluetoothServicesDBusMonitor, err := log.StartDBusMonitorCollector(
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
		var extraArgs []string
		if tf.fastPairEnabled {
			extraArgs = fixtureVarFastPairExtraArgs
		}
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
			ExtraArgs:                    extraArgs,
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
	if err := tf.dumpAllCollectedLogs(ctx, "SetUp"); err != nil {
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
	if err := tf.dumpAllCollectedLogs(ctx, "Reset"); err != nil {
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
	if err := tf.dumpAllCollectedLogs(ctx, "PostTest"); err != nil {
		s.Fatal("Failed to collect dbus-monitor bluez logs: ", err)
	}
}

// TearDown is called by the framework to tear down the environment SetUp set
// up.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// Reset btpeers to original state and shut down tunnels.
	if err := tf.resetBTPeers(ctx); err != nil {
		s.Error("Failed to reset all btpeers: ", err)
	}
	for _, btpeerCompanion := range tf.fv.bTPeerCompanions {
		if err := btpeerCompanion.chameleondPortForwarder.Close(); err != nil {
			s.Errorf("Failed to shut down forwarded chameleond port tunnel for btpeer %q: %v", btpeerCompanion.host, btpeerCompanion)
		}
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
	if err := tf.dumpAllCollectedLogs(ctx, "TearDown"); err != nil {
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

	// Resolve the btpeer hosts.
	var btpeerHosts []string
	if btpeersVar, isSet := s.Var(fixtureVarBTPeers); isSet && btpeersVar != "" {
		btpeerHosts = strings.Split(btpeersVar, ",")
		if len(btpeerHosts) < requiredBTPeers {
			return errors.Errorf("fixture requires at least %d btpeers, but "+
				"only %d were provided in the %s tast var (%q)",
				requiredBTPeers, len(btpeerHosts),
				fixtureVarBTPeers, btpeersVar)
		}
		btpeerHosts = btpeerHosts[:requiredBTPeers]
	} else {
		// Imply btpeer hostnames based on DUT hostname.
		btpeerHosts = make([]string, requiredBTPeers)
		dutHostname := strings.Split(s.DUT().HostName(), ":")[0]
		if dutHostname == "localhost" || dutHostname == "" || dutHostname == "127.0.0.1" {
			for i := 0; i < requiredBTPeers; i++ {
				btpeerHosts[i] = fmt.Sprintf("localhost:%d", 2201+i)
			}
			exampleTastCall := fmt.Sprintf("tast run --var=%s=%s %s <test>",
				fixtureVarBTPeers, strings.Join(btpeerHosts, ","),
				s.DUT().HostName())
			return errors.Errorf("btpeer hostname resolution not supported "+
				"when DUT hostname is %q. If tast is being run in a local "+
				"development environment outside of the lab, ssh tunnel to the "+
				"btpeers outside of the chroot and provide the local forwarded"+
				" address to tast using the %q tast var (e.g. %q)",
				dutHostname, fixtureVarBTPeers, exampleTastCall)
		}
		for i := 0; i < requiredBTPeers; i++ {
			btpeerNum := i + 1
			btpeerHostnameSuffix := fmt.Sprintf("-btpeer%d", btpeerNum)
			btpeerHostname, err := utils.CompanionDeviceHostname(s.DUT().HostName(), btpeerHostnameSuffix)
			if err != nil {
				return errors.Wrapf(err, "failed to build companion device hostname for btpeer%d", btpeerNum)
			}
			btpeerHosts[i] = btpeerHostname
		}
	}

	// Connect to btpeers over ssh and access chameleond over through a tunnel.
	testing.ContextLogf(ctx, "Connecting to %d btpeers: %s",
		len(btpeerHosts), strings.Join(btpeerHosts, ", "))
	for _, host := range btpeerHosts {
		// Connect to btpeer over ssh using standard test credentials.
		sshOptions := &ssh.Options{
			KeyDir:  s.DUT().KeyDir(),
			KeyFile: s.DUT().KeyFile(),
		}
		if err := ssh.ParseTarget(host, sshOptions); err != nil {
			return errors.Wrapf(err, "failed to parse ssh target btpeer host %q", host)
		}
		sshConn, err := ssh.New(ctx, sshOptions)
		if err != nil {
			return errors.Wrapf(err, "failed to connect to btpeer host %q over ssh", host)
		}

		var logCollector *log.JournalctlCollector
		var chameleondPortForwarder *ssh.Forwarder
		prepareBTPeerForChameleond := func() error {
			var err error

			// Start collecting chameleond logs on the btpeer from chameleond.
			logCollector, err = log.StartJournalctlCollector(ctx, sshConn, "--output", "short-full")
			if err != nil {
				return errors.Wrapf(err, "failed to start collecting chameleond logs on btpeer host %q", host)
			}

			// Port forward chameleond port.
			onFwdError := func(err error) {
				testing.ContextLogf(ctx, "ssh forwarding error for btpeer host %q: %v", host, err)
			}

			chameleondPortForwarder, err = sshConn.ForwardLocalToRemote("tcp", "localhost:0", "localhost:9992", onFwdError)
			if err != nil {
				return errors.Wrapf(err, "failed to port forward chameleond port for btpeer host %q", host)
			}
			return nil
		}
		if err := prepareBTPeerForChameleond(); err != nil {
			return err
		}

		// Connect chameleond client to forwarded port. Reboot once if the first try
		// fails, as sometimes the btpeer can be left in an unstable state.
		testing.ContextLogf(ctx, "Connecting to chameleond on btpeer host %q through forwarded chameleond port at %q", host, chameleondPortForwarder.ListenAddr().String())
		chameleondClient, err := chameleon.NewChameleond(ctx, chameleondPortForwarder.ListenAddr().String())
		if err != nil {
			testing.ContextLogf(ctx, "Initial chameleond connection attempt for btpeer host %q failed, rebooting btpeer and retrying", host)

			// Reboot, ignoring the ssh error that occurs due to severed connection.
			_ = logCollector.Close()
			_ = sshConn.CommandContext(ctx, "reboot").Run()

			// Try to reconnect via ssh until successful.
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				var err error
				sshConn, err = ssh.New(ctx, sshOptions)
				if err != nil {
					return errors.Wrapf(err, "failed to reconnect to btpeer host %q over ssh after reboot", host)
				}
				return nil
			}, &testing.PollOptions{
				Interval: 1 * time.Second,
				Timeout:  1 * time.Minute,
			}); err != nil {
				return err
			}

			if err := prepareBTPeerForChameleond(); err != nil {
				return err
			}

			// Try chameleond again with a short poll as ssh may come up before
			// chameleond does.
			testing.ContextLogf(ctx, "Connecting to chameleond on btpeer host %q through forwarded chameleond port at %q after reboot", host, chameleondPortForwarder.ListenAddr().String())
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				var err error
				chameleondClient, err = chameleon.NewChameleond(ctx, chameleondPortForwarder.ListenAddr().String())
				return err
			}, &testing.PollOptions{
				Interval: 500 * time.Millisecond,
				Timeout:  10 * time.Second,
			}); err != nil {
				return errors.Wrapf(err, "failed to connect to chameleond on btpeer host %q through forward chameleond port at %q", host, chameleondPortForwarder.ListenAddr().String())
			}
		}

		// Save btpeer companion for later use.
		btpeerCompanion := &bTPeerCompanion{
			host:                    host,
			sshConn:                 sshConn,
			chameleondClient:        chameleondClient,
			chameleondPortForwarder: chameleondPortForwarder,
			logCollector:            logCollector,
		}
		tf.fv.bTPeerCompanions = append(tf.fv.bTPeerCompanions, btpeerCompanion)
		tf.fv.BTPeers = append(tf.fv.BTPeers, btpeerCompanion.chameleondClient)
	}

	testing.ContextLogf(ctx, "Successfully connected to %d btpeers", len(tf.fv.BTPeers))
	return nil
}

// resetBTPeers resets each configured btpeer to return them to their normal
// state and clear any changes a test may have made to them.
// Each btpeer is reset in parallel to save time. If any reset fails, the first
// error is returned and any pending resets are cancelled.
func (tf *fixture) resetBTPeers(ctx context.Context) error {
	ctx, st := timing.Start(ctx, fmt.Sprintf("resetBTPeers_%d", len(tf.fv.bTPeerCompanions)))
	defer st.End()
	if len(tf.fv.bTPeerCompanions) == 0 {
		return nil
	}
	testing.ContextLogf(ctx, "Resetting %d btpeers", len(tf.fv.bTPeerCompanions))
	resetCtx, cancelResetCtx := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelResetCtx()
	resetGroup, resetCtx := errgroup.WithContext(resetCtx)
	for i, btpeerCompanion := range tf.fv.bTPeerCompanions {
		// Note: loop var values are copied to inner vars for use in func literal.
		i := i
		btpeerCompanion := btpeerCompanion
		resetGroup.Go(func() error {
			return tf.resetBTPeer(resetCtx, i, btpeerCompanion)
		})
	}
	if err := resetGroup.Wait(); err != nil {
		return errors.Wrap(err, "failed to reset btpeers")
	}
	return nil
}

func (tf *fixture) resetBTPeer(ctx context.Context, btpeerIndex int, btpeerCompanion *bTPeerCompanion) error {
	// Reset the base chameleond service state.
	if err := btpeerCompanion.chameleondClient.Reset(ctx); err != nil {
		return errors.Wrapf(err, "failed to reset chameleond on btpeer[%d] at %q", btpeerIndex, btpeerCompanion.host)
	}
	// Reset the bluetooth service state, through the keyboard device interface
	// since this method is not exposed at a higher level.
	if err := btpeerCompanion.chameleondClient.BluetoothKeyboardDevice().ResetStack(ctx, ""); err != nil {
		return errors.Wrapf(err, "failed to reset bluetooth stack on btpeer[%d] at %q", btpeerIndex, btpeerCompanion.host)
	}
	return nil
}

func (tf *fixture) dumpAllCollectedLogs(ctx context.Context, logName string) error {
	ctx, st := timing.Start(ctx, "dumpAllCollectedLogs")
	defer st.End()
	for i, dbusMonitor := range tf.bluetoothServicesDBusMonitors {
		dutName := fmt.Sprintf("dut%d", i)
		logDir := filepath.Join("dbus_monitor_bluetooth", dutName)
		if err := log.DumpCollectedLogsToFile(ctx, dbusMonitor, logDir, logName); err != nil {
			return errors.Wrapf(err, "failed to dump collected dbus-monitor messages from %s", dutName)
		}
	}
	for i, btpeer := range tf.fv.bTPeerCompanions {
		btpeerName := fmt.Sprintf("btpeer%d", i+1)
		logDir := filepath.Join("btpeer_system_logs", btpeerName)
		if err := log.DumpCollectedLogsToFile(ctx, btpeer.logCollector, logDir, logName); err != nil {
			return errors.Wrapf(err, "failed to dump collected btpeer system logs from %s at %q", btpeerName, btpeerName)
		}
	}
	return nil
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
