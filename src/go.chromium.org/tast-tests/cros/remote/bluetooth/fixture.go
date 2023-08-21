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

	cryptossh "golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/log"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	qs "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
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

// Public test variable keys that are used in multiple tests.
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

// Tast services.
const (
	serviceDepBluetoothService     = "tast.cros.bluetooth.BluetoothService"
	serviceDepBluetoothUIService   = "tast.cros.bluetooth.BluetoothUIService"
	serviceDepUpstartService       = "tast.cros.platform.UpstartService"
	serviceDepAudioService         = "tast.cros.ui.AudioService"
	serviceDepChromeService        = "tast.cros.browser.ChromeService"
	serviceDepQuickSettingsService = "tast.cros.chrome.uiauto.quicksettings.QuickSettingsService"
)

// DUT D-Bus services.
const (
	dbusServiceBluetoothBluez        = "org.bluez"
	dbusServiceBluetoothFloss        = "org.chromium.bluetooth"
	dbusServiceBluetoothFlossManager = "org.chromium.bluetooth.Manager"
)

// Chrome features.
const (
	chromeFeatureOobeHidDetectionRevamp = "OobeHidDetectionRevamp"
	chromeFeatureFastPair               = "FastPair"
	chromeFeatureFastPairSavedDevices   = "FastPairSavedDevices"

	// chromeFeatureFloss is enabled when FlossEnabled fixture feature is true,
	// and disabled when it is false.
	chromeFeatureFloss = "Floss"
)

// Fixture timeouts.
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

const (
	btpeerVersionLogFilePath    = "/var/log/chameleon_commits"
	btpeerChameleondLogFilePath = "/var/log/chameleond"
)

type fixtureFeatures struct {
	// EnableChromeUI will ensure that chrome UI is enabled during the test if
	// true, or disabled if false.
	EnableChromeUI bool

	// BTPeerCount requires the specified amount of btpeers to exist in the
	// testbed and connects to them during setup. A testbed can have more btpeers
	// than the BTPeerCount, but only that many connections are configured.
	BTPeerCount int

	// EnableFeatures is the list of features that will be enabled when starting Chrome.
	EnableFeatures []string

	// DisableFeatures is the list of features that will be enabled when starting Chrome.
	DisableFeatures []string

	// LoginMode is what the resulting login mode should be after starting Chrome.
	LoginMode ui.LoginMode

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

	// FlossEnabled allows for switching the bluetooth stack on the DUT to use floss
	// or bluez. To use floss, set this to "true", otherwise bluez will be used.
	FlossEnabled bool
}

// DUTConfig groups DUT-specific fixture configs and utils.
type DUTConfig struct {
	// DUT is the connection to the primary DUT.
	DUT *dut.DUT

	// DUTRPCClient is a gRPC client that remains connected to the DUT throughout
	// the life of the test fixture. This can be used to create clients to
	// additional local tast services.
	DUTRPCClient *rpc.Client

	// BluetoothService is a client of the BluetoothService that is used to
	// interact and manage the bluetooth stack on the DUT.
	//
	// Note: Functions that modify the adapter state should only be used in
	// tests where the chrome UI is disabled to avoid interference with the
	// bluetooth daemon it runs. For UI testing, the BluetoothUIService should
	// be used instead.
	BluetoothService bts.BluetoothServiceClient

	// BluetoothUIService is a client of the BluetoothUIService that uses the
	// DUTRPCClient connection.
	BluetoothUIService bts.BluetoothUIServiceClient

	// ChromeService is a client of the ChromeService that is used to start Chrome.
	ChromeService ui.ChromeServiceClient

	// UpstartService is a client of the UpstartService that manages system jobs.
	UpstartService platform.UpstartServiceClient

	// AudioService is a client of the AudioService that manages audio settings.
	AudioService ui.AudioServiceClient

	// QuickSettingsService is a client of the QuickSettingsService that manages
	// UI related services in the quick settings.
	QuickSettingsService qs.QuickSettingsServiceClient
}

func newDUTConfig(ctx context.Context, dut *dut.DUT, RPCHint *testing.RPCHint) (*DUTConfig, error) {
	rpcClient, err := rpc.Dial(ctx, dut, RPCHint)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to the local gRPC service on DUT %s", dut.HostName())
	}
	return &DUTConfig{
		DUT:                  dut,
		DUTRPCClient:         rpcClient,
		BluetoothService:     bts.NewBluetoothServiceClient(rpcClient.Conn),
		BluetoothUIService:   bts.NewBluetoothUIServiceClient(rpcClient.Conn),
		ChromeService:        ui.NewChromeServiceClient(rpcClient.Conn),
		UpstartService:       platform.NewUpstartServiceClient(rpcClient.Conn),
		AudioService:         ui.NewAudioServiceClient(rpcClient.Conn),
		QuickSettingsService: qs.NewQuickSettingsServiceClient(rpcClient.Conn),
	}, nil
}

type bTPeerCompanion struct {
	host                    string
	sshConn                 *ssh.Conn
	chameleondClient        chameleon.Chameleond
	chameleondPortForwarder *ssh.Forwarder
	systemLogCollector      log.Collector
	chameleondLogCollector  log.Collector
	chameleondLastCommit    string
	chameleondUpdatedAt     string
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

	// BluetoothService is a client of the BluetoothService that is used to
	// interact and manage the bluetooth stack on the DUT.
	//
	// Note: Functions that modify the adapter state should only be used in
	// tests where the chrome UI is disabled to avoid interference with the
	// bluetooth daemon it runs. For UI testing, the BluetoothUIService should
	// be used instead.
	BluetoothService bts.BluetoothServiceClient

	// BluetoothUIService is a client of the BluetoothUIService that uses the
	// DUTRPCClient connection.
	BluetoothUIService bts.BluetoothUIServiceClient

	// ChromeService is a client of the ChromeService that is used to start Chrome.
	ChromeService ui.ChromeServiceClient

	// AudioService is a client of the AudioService that is used to configure Audio.
	AudioService ui.AudioServiceClient

	// QuickSettingsService is a client of QuickSettingsService to access the UI.
	QuickSettingsService qs.QuickSettingsServiceClient

	// DUTs stores the dut-specific configurations for each DUT in the fixture.
	// The first item in this list refers to the primary DUT, and subsequent items
	// refer to companion DUTs.
	DUTConfigs []*DUTConfig

	// UpstartService is a client of the UpstartService that manages system jobs.
	UpstartService platform.UpstartServiceClient
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
	// Persistent vars, set just once in newFixture.
	features        *fixtureFeatures
	fastPairEnabled bool

	// Stateful vars which are initialized during SetUp.
	fv                            *FixtValue
	bluetoothServicesDBusMonitors []*log.DBusMonitorCollector
}

func newFixture(features *fixtureFeatures) *fixture {
	tf := &fixture{
		features: features,
	}
	// Determine if fast pair is an enabled feature for later reference.
	tf.fastPairEnabled = false
	for _, feature := range tf.features.EnableFeatures {
		if feature == chromeFeatureFastPairSavedDevices {
			tf.fastPairEnabled = true
			break
		}
	}
	return tf
}

// SetUp preforms fixture setup actions. All fixtureFeatures are configured.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Ensure any stateful fixture properties are set to initial state.
	tf.fv = &FixtValue{}
	tf.bluetoothServicesDBusMonitors = nil

	// Determine desired bluetooth stack for DUTs.
	var btStack bts.BluetoothStackType
	if tf.features.FlossEnabled {
		btStack = bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS
	} else {
		btStack = bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ
	}

	// Parse OOBE fixture var.
	var signinProfileTestExtensionID string
	if tf.features.EnableHidScreenOnOobe {
		var ok bool
		signinProfileTestExtensionID, ok = s.Var(fixtureVarSigninKey)
		if !ok {
			s.Fatal("Failed to get sign-in key variable required for OOBE tests")
		}
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
	tf.fv.BluetoothUIService = primaryDUTConfig.BluetoothUIService
	tf.fv.BluetoothService = primaryDUTConfig.BluetoothService
	tf.fv.ChromeService = primaryDUTConfig.ChromeService
	tf.fv.UpstartService = primaryDUTConfig.UpstartService
	tf.fv.AudioService = primaryDUTConfig.AudioService
	tf.fv.QuickSettingsService = primaryDUTConfig.QuickSettingsService

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
			fmt.Sprintf("sender='%s'", dbusServiceBluetoothFlossManager),
			fmt.Sprintf("destination='%s'", dbusServiceBluetoothBluez),
			fmt.Sprintf("destination='%s'", dbusServiceBluetoothFloss),
			fmt.Sprintf("destination='%s'", dbusServiceBluetoothFlossManager),
		)
		if err != nil {
			s.Fatal("Failed to start dbus-monitor listening to bluez and floss service messages: ", err)
		}
		tf.bluetoothServicesDBusMonitors = append(tf.bluetoothServicesDBusMonitors, bluetoothServicesDBusMonitor)

		// Enable/Disable floss feature based on desired bluetooth stack.
		if btStack == bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS {
			tf.features.EnableFeatures = append(tf.features.EnableFeatures, chromeFeatureFloss)
		} else {
			tf.features.DisableFeatures = append(tf.features.DisableFeatures, chromeFeatureFloss)
		}

		if tf.features.EnableChromeUI {
			// Resolve chrome user credentials.
			var chromeUsername, chromePassword string
			s.Log("Resolving chrome user credentials")
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

			// Start Chrome with the features and login mode provided by the test fixture.
			var extraArgs []string
			if tf.fastPairEnabled {
				extraArgs = fixtureVarFastPairExtraArgs
			}
			if _, err := dutConfig.ChromeService.New(ctx, &ui.NewRequest{
				LoginMode:       tf.features.LoginMode,
				EnableFeatures:  tf.features.EnableFeatures,
				DisableFeatures: tf.features.DisableFeatures,
				Credentials: &ui.NewRequest_Credentials{
					Username: chromeUsername,
					Password: chromePassword,
				},
				EnableHidScreenOnOobe:        tf.features.EnableHidScreenOnOobe,
				SigninProfileTestExtensionId: signinProfileTestExtensionID,
				ExtraArgs:                    extraArgs,
			}); err != nil {
				s.Fatalf("Failed to log into chrome on DUT %s: %v", dutName, err)
			}
		} else {
			s.Logf("Stopping Chrome UI job on DUT %s", dutName)
			if _, err := dutConfig.UpstartService.StopJob(ctx, &platform.StopJobRequest{
				JobName: "ui",
			}); err != nil {
				s.Fatalf("Failed to stop Chrome UI job on DUT %s: %v", dutName, err)
			}
		}

		// Configure and enable desired DUT bluetooth stack.
		s.Logf("Configuring DUT %s bluetooth stack to use %s", dutName, btStack.String())
		if _, err := dutConfig.BluetoothService.SetBluetoothStack(ctx, &bts.SetBluetoothStackRequest{
			StackType: btStack,
		}); err != nil {
			s.Fatalf("Failed to configure DUT %s bluetooth stack as %s: %v", dutName, btStack.String(), err)
		}
		s.Logf("Enabling bluetooth on DUT %s", dutName)
		if _, err := dutConfig.BluetoothService.Enable(ctx, &emptypb.Empty{}); err != nil {
			s.Fatalf("Failed to enable %s bluetooth stack on DUT %s: %v", btStack.String(), dutName, err)
		}
		if err := tf.resetDutBluetoothState(ctx, dutConfig, true); err != nil {
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
		if err := tf.resetDutBluetoothState(ctx, dutConfig, true); err != nil {
			return errors.Wrapf(err, "failed to reset state of DUT %s", dutConfig.DUT.HostName())
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
		if err := tf.resetDutBluetoothState(ctx, dutConfig, true); err != nil {
			s.Errorf("Failed to reset state of DUT %s: %v", dutName, err)
		}
		s.Logf("Disabling bluetooth on DUT %s", dutName)
		if _, err := dutConfig.BluetoothService.Disable(ctx, &emptypb.Empty{}); err != nil {
			s.Errorf("Failed to disable bluetooth stack on DUT %s: %v", dutName, err)
		}

		if tf.features.EnableChromeUI {
			// Clean up chrome login state.
			if _, err := dutConfig.ChromeService.Close(ctx, &emptypb.Empty{}); err != nil {
				s.Error("Failed to close Chrome on the DUT: ", err)
			}
		}

		// Close gRPC connection to DUT.
		if err := dutConfig.DUTRPCClient.Close(ctx); err != nil {
			s.Error("Failed to close gRPC connection to DUT: ", err)
		}

		s.Logf("TearDown for DUT %s completed", dutName)
	}

	// Dump and close log collectors.
	if err := tf.dumpAllCollectedLogs(ctx, "TearDown"); err != nil {
		s.Error("Failed to collect dbus-monitor bluez logs: ", err)
	}
	for _, dbusMonitor := range tf.bluetoothServicesDBusMonitors {
		if err := dbusMonitor.Close(); err != nil {
			s.Error("Failed to close dbus-monitor: ", err)
		}
	}
	for i, bTPeerCompanion := range tf.fv.bTPeerCompanions {
		if err := bTPeerCompanion.systemLogCollector.Close(); err != nil {
			s.Errorf("Failed to close system log collector on btpeer%d: %v", i+1, err)
		}
		if bTPeerCompanion.chameleondLogCollector != nil {
			if err := bTPeerCompanion.chameleondLogCollector.Close(); err != nil {
				s.Errorf("Failed to close chameleond log collector on btpeer%d: %v", i+1, err)
			}
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

		var systemLogCollector *log.JournalctlCollector
		var chameleondLogCollector *log.TailCollector
		var chameleondPortForwarder *ssh.Forwarder
		prepareBTPeerForChameleond := func() error {
			var err error

			// Start collecting system and chameleond logs on the btpeer.
			systemLogCollector, err = log.StartJournalctlCollector(ctx, sshConn, "--output", "short-full")
			if err != nil {
				return errors.Wrapf(err, "failed to start collecting system logs on btpeer host %q", host)
			}
			hasChameleondLogFile, err := remoteFileExists(ctx, sshConn, btpeerChameleondLogFilePath)
			if err != nil {
				return errors.Wrapf(err, "failed to check for chameleond log file %q on btpeer host %q", btpeerChameleondLogFilePath, host)
			}
			if hasChameleondLogFile {
				chameleondLogCollector, err = log.StartTailCollector(ctx, sshConn, btpeerChameleondLogFilePath, true)
				if err != nil {
					return errors.Wrapf(err, "failed to start collecting chameleond logs on btpeer host %q", host)
				}
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
			if systemLogCollector != nil {
				_ = systemLogCollector.Close()
			}
			if chameleondLogCollector != nil {
				_ = chameleondLogCollector.Close()
			}
			if chameleondPortForwarder != nil {
				_ = chameleondPortForwarder.Close()
			}
			return err
		}

		// Connect chameleond client to forwarded port. Reboot once if the first try
		// fails, as sometimes the btpeer can be left in an unstable state.
		testing.ContextLogf(ctx, "Connecting to chameleond on btpeer host %q through forwarded chameleond port at %q", host, chameleondPortForwarder.ListenAddr().String())
		chameleondClient, err := chameleon.NewChameleond(ctx, chameleondPortForwarder.ListenAddr().String())
		if err != nil {
			testing.ContextLogf(ctx, "Initial chameleond connection attempt for btpeer host %q failed, rebooting btpeer and retrying", host)

			// Reboot, ignoring the ssh error that occurs due to severed connection.
			_ = systemLogCollector.Close()
			_ = chameleondLogCollector.Close()
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
				if systemLogCollector != nil {
					_ = systemLogCollector.Close()
				}
				if chameleondLogCollector != nil {
					_ = chameleondLogCollector.Close()
				}
				if chameleondPortForwarder != nil {
					_ = chameleondPortForwarder.Close()
				}
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

		// Attempt to fetch the chameleond version (not supported on old versions).
		var chameleondLastCommit, chameleondUpdatedAt string
		btpeerVersionLogFileExists, err := remoteFileExists(ctx, sshConn, btpeerVersionLogFilePath)
		if err != nil {
			return errors.Wrapf(err, "failed to check for chameleond log file %q on btpeer host %q", btpeerChameleondLogFilePath, host)
		}
		if btpeerVersionLogFileExists {
			lastLogLine, err := sshConn.CommandContext(ctx, "tail", "-1", btpeerVersionLogFilePath).Output()
			if err == nil {
				lastLogLineParts := strings.Split(strings.TrimSpace(string(lastLogLine)), " ")
				if len(lastLogLineParts) == 2 {
					chameleondLastCommit = lastLogLineParts[0]
					chameleondUpdatedAt = lastLogLineParts[1]
				}
			}
		}
		if chameleondLastCommit == "" {
			chameleondLastCommit = "unknown"
		}
		if chameleondUpdatedAt == "" {
			chameleondUpdatedAt = "unknown"
		}

		// Save btpeer companion for later use.
		btpeerCompanion := &bTPeerCompanion{
			host:                    host,
			sshConn:                 sshConn,
			chameleondClient:        chameleondClient,
			chameleondPortForwarder: chameleondPortForwarder,
			systemLogCollector:      systemLogCollector,
			chameleondLogCollector:  chameleondLogCollector,
			chameleondLastCommit:    chameleondLastCommit,
			chameleondUpdatedAt:     chameleondUpdatedAt,
		}
		tf.fv.bTPeerCompanions = append(tf.fv.bTPeerCompanions, btpeerCompanion)
		tf.fv.BTPeers = append(tf.fv.BTPeers, btpeerCompanion.chameleondClient)
	}

	testing.ContextLogf(ctx, "Successfully connected to %d btpeers", len(tf.fv.bTPeerCompanions))
	for i, btpeer := range tf.fv.bTPeerCompanions {
		testing.ContextLogf(ctx, "Chameleond on btpeer%d was last updated at %q to commit %q", i+1, btpeer.chameleondUpdatedAt, btpeer.chameleondLastCommit)
	}
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
		baseLogDir := filepath.Join("btpeer_logs", btpeerName)
		systemLogDir := filepath.Join(baseLogDir, "system")
		chameleondLogDir := filepath.Join(baseLogDir, "chameleond")
		if err := log.DumpCollectedLogsToFile(ctx, btpeer.systemLogCollector, systemLogDir, logName); err != nil {
			return errors.Wrapf(err, "failed to dump collected btpeer system logs from btpeer %q", btpeerName)
		}
		if btpeer.chameleondLogCollector != nil {
			if err := log.DumpCollectedLogsToFile(ctx, btpeer.chameleondLogCollector, chameleondLogDir, logName); err != nil {
				return errors.Wrapf(err, "failed to dump collected btpeer chameleond logs from btpeer %q", btpeerName)
			}
		}
	}
	return nil
}

// resetDutBluetoothState resets the bluetooth state of the DUT so that it is
// ready for tests.
func (tf *fixture) resetDutBluetoothState(ctx context.Context, dutConfig *DUTConfig, enableBluetooth bool) error {
	dutName := dutConfig.DUT.HostName()
	// Handle Fast Pair UI reset needs.
	if tf.fastPairEnabled {
		testing.ContextLogf(ctx, "Removing all saved bluetooth devices via UI on DUT %s", dutName)
		if _, err := dutConfig.BluetoothUIService.RemoveAllSavedDevices(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to remove all saved bluetooth devices via UI on DUT %s", dutName)
		}
		testing.ContextLogf(ctx, "Closing all UI notifications on DUT %s", dutName)
		if _, err := dutConfig.BluetoothUIService.CloseNotifications(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrapf(err, "failed to close all UI notifications on DUT %s", dutName)
		}
	}

	// Reset the state of the bluetooth adapter.
	testing.ContextLogf(ctx, "Resetting and setting bluetooth enabled to %t on DUT %s", enableBluetooth, dutName)
	if _, err := dutConfig.BluetoothService.Reset(ctx, &bts.ResetRequest{
		PowerOn: enableBluetooth,
	}); err != nil {
		return errors.Wrapf(err, "failed to reset and set bluetooth enabled to %t on DUT %s", enableBluetooth, dutName)
	}
	return nil
}

// remoteFileExists runs the `test -f <path>` command using the provided ssh
// connection to verify file existence. Returns true if the test passes and
// false if the test fails. A non-nil error is returned if the command fails
// to run as expected.
func remoteFileExists(ctx context.Context, sshConn *ssh.Conn, path string) (bool, error) {
	if err := sshConn.CommandContext(ctx, "test", "-f", path).Run(); err != nil {
		exitErr, ok := err.(*cryptossh.ExitError)
		if !ok || exitErr.ExitStatus() != 1 {
			return false, errors.Wrapf(err, "failed to run 'test -f %q' on remote host", path)
		}
		return false, nil
	}
	return true, nil
}
