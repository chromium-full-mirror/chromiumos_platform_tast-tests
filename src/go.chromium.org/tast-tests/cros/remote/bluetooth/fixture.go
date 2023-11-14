// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/log"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	qs "go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"

	// for power measurement
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/services/cros/power"

	"github.com/golang/protobuf/ptypes/empty"
)

// Fixture variable keys.
const (
	// fixtureVarBTPeers is the name of the tast var that specifies a
	// comma-separated list of btpeer hostname addresses.
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
	serviceDepMetricsService       = "tast.cros.power.MetricsService"
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

	// chromeFeatureFlossIsAvailabilityCheckNeeded needs to be disabled when
	// chromeFeatureFloss is enabled.
	chromeFeatureFlossIsAvailabilityCheckNeeded = "FlossIsAvailabilityCheckNeeded"
)

// Power measurement parameters.
const (
	DefaultBTPowerIntervalSecond = 5
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

	// UseSameGaiaLogin enforces that all companion DUTs will login on the same account
	// as the primary DUT.
	UseSameGaiaLogin bool

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

	// PowerEnabled allows for power measurement
	PowerEnabled bool
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

	// PowerMetricsService is a client of the MetricsService that measures the power
	// consumption.
	PowerMetricsService power.MetricsServiceClient
}

func newDUTConfig(ctx context.Context, dut *dut.DUT, RPCHint *testing.RPCHint) (*DUTConfig, error) {
	rpcClient, err := rpc.Dial(ctx, dut, RPCHint)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to Connect to the local gRPC service on DUT %s", dut.HostName())
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
		PowerMetricsService:  power.NewMetricsServiceClient(rpcClient.Conn),
	}, nil
}

// FixtValue is the value of the test fixture accessible within a test. All
// variables are configured in fixture.SetUp so that tests can use them without
// any further configuration.
type FixtValue struct {
	// BTPeers is a list of btpeer clients that are connected to each btpeer
	// available to the test fixture.
	BTPeers []*BtpeerClient

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

	// PowerMetricsService is a client of MetricsService to record the power consumption.
	PowerMetricsService power.MetricsServiceClient
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

// StartPowerRecording to start a recording for power metrics.
func (fv *FixtValue) StartPowerRecording(ctx context.Context) error {
	testing.ContextLog(ctx, "Start recording power metrics")
	if _, err := fv.PowerMetricsService.Start(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start recording power metrics")
	}
	return nil
}

// StopPowerRecording to stop the existing power recording.
func (fv *FixtValue) StopPowerRecording(ctx context.Context, uploadTestName string) (*perf.Values, error) {

	path, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get OutDir")
	}
	remotePath := filepath.Join(path, "power_metrics")

	testing.ContextLog(ctx, "Stop recording power metrics")
	request := power.FinishRequest{Upload: true, OutDir: remotePath, TestName: uploadTestName}
	values, err := fv.PowerMetricsService.Finish(ctx, &request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stop recording power metrics")
	}

	for i, dutConfig := range fv.DUTConfigs {
		dut := dutConfig.DUT
		powerDir := fmt.Sprintf("power_dut%d_%s", i, uploadTestName)
		logPathDst := filepath.Join(path, powerDir)
		if err := linuxssh.GetFile(ctx, dut.Conn(), remotePath, logPathDst, linuxssh.PreserveSymlinks); err != nil {
			testing.ContextLogf(ctx, "Failed to copy %s from DUT to local path: %s. Error: %s", remotePath, logPathDst, err)
		}
		// Delete the log files from the DUT so that we have a clean run.
		if err := dut.Conn().CommandContext(ctx, "rm", "-r", remotePath).Run(); err != nil {
			testing.ContextLogf(ctx, "Failed to remove the log files from the DUT at the end of test: %s", err)
		}
	}

	// Convert perfpb.Values to perf.Values
	return perf.NewValuesFromProto(values), nil
}

// GetPowerMetrics calculates the mean value of requested power metrics.
func (fv *FixtValue) GetPowerMetrics(ctx context.Context, powerResults *perf.Values, metricsName string) (float64, error) {

	var powerMean float64 = -1
	for key, value := range powerResults.GetValues() {
		if key.Name == metricsName {
			if len(value) == 0 {
				return -1, errors.New("measurement is empty")
			}
			var total float64 = 0
			for _, v := range value {
				total += v
			}
			mean := total / float64(len(value))
			var powerSd float64 = 0
			for _, v := range value {
				powerSd += math.Pow(v-mean, 2)
			}
			powerSd = math.Sqrt(powerSd / float64(len(value)))
			testing.ContextLogf(ctx, "\t%s=%.4f sd=%.4f n=%d", key.Name, mean, powerSd, len(value))
			powerMean = mean
		}
	}
	if powerMean == -1 {
		return -1, errors.New("no power measurement found")
	}
	return powerMean, nil
}

type fixture struct {
	// Persistent vars, set just once in newFixture.
	features        *fixtureFeatures
	fastPairEnabled bool

	// Stateful vars which are initialized during SetUp.
	fv                            *FixtValue
	bluetoothServicesDBusMonitors []*log.DBusMonitorCollector
	btsnoopCollectors             []*log.BtsnoopCollector
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
	tf.btsnoopCollectors = nil

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
	tf.fv.PowerMetricsService = primaryDUTConfig.PowerMetricsService

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

	// Cleanup if anything goes wrong during Setup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Perform per-DUT setup actions.
	var primaryDUTUsername, primaryDUTPassword string
	for dutIndex, dutConfig := range tf.fv.DUTConfigs {
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

		btsnoopCollector, err := log.StartBtsnoopCollector(ctx, dutConfig.DUT.Conn())
		if err != nil {
			s.Fatal("Failed to start btsnoop log: ", err)
		}
		tf.btsnoopCollectors = append(tf.btsnoopCollectors, btsnoopCollector)

		// Enable/Disable floss feature based on desired bluetooth stack.
		if btStack == bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS {
			tf.features.EnableFeatures = append(tf.features.EnableFeatures, chromeFeatureFloss)
			tf.features.DisableFeatures = append(tf.features.DisableFeatures, chromeFeatureFlossIsAvailabilityCheckNeeded)
		} else {
			tf.features.DisableFeatures = append(tf.features.DisableFeatures, chromeFeatureFloss)
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

		if tf.features.EnableChromeUI {
			// Resolve chrome user credentials.
			var chromeUsername, chromePassword string
			s.Log("Resolving chrome user credentials")
			if tf.features.UseSameGaiaLogin && dutIndex != 0 {
				// All companion DUTs (expected to be in the last half of the config list) should
				// use the same GAIA credentials as the first account.
				chromeUsername = primaryDUTUsername
				chromePassword = primaryDUTPassword
			} else if tf.features.RequireFastPairUserVars {
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

			if tf.features.UseSameGaiaLogin && dutIndex == 0 {
				// Save primary DUT credentials if we have to re-use them for companion DUT logins.
				primaryDUTUsername = chromeUsername
				primaryDUTPassword = chromePassword
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

		if err := tf.resetDutBluetoothState(ctx, dutConfig, true); err != nil {
			s.Errorf("Failed to reset state of DUT %s: %v", dutName, err)
		}

		// Set up power test
		if tf.features.PowerEnabled {
			// Create message SetupRequest
			s.Log("Set up for power measurement")
			setupRequest := power.SetupRequest{Fixture: power.SetupRequest_NO_UI_NO_WIFI_BT,
				IntervalSecond: DefaultBTPowerIntervalSecond}

			if tf.features.EnableChromeUI {
				setupRequest = power.SetupRequest{Fixture: power.SetupRequest_UI_NO_WIFI_BT,
					IntervalSecond: DefaultBTPowerIntervalSecond}
			}

			if _, err := dutConfig.PowerMetricsService.Setup(ctx, &setupRequest); err != nil {
				s.Fatal("Failed to set up metrics service: ", err)
			}

			defer func(ctx context.Context) {
				if !s.HasError() {
					return
				}
				if _, err := dutConfig.PowerMetricsService.Cleanup(ctx, &empty.Empty{}); err != nil {
					s.Error("Clean up power metrics failed: ", err)
				}
			}(cleanupCtx)

			s.Logf("Set up of power measurement for DUT %s completed", dutName)
		}

		if btStack == bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ {
			if _, err = tf.fv.BluetoothService.SetDebugLogLevels(ctx, &bts.SetDebugLogLevelsRequest{Level: 1}); err != nil {
				testing.ContextLog(ctx, "Failed to set log level: ", err)
			}
		}

		s.Logf("Set up for DUT %s completed", dutName)
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
	if err := GetBtpeerProvider().Reset(ctx, tf.fv.BTPeers...); err != nil {
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
	// Reset btpeers.
	if err := GetBtpeerProvider().Reset(ctx, tf.fv.BTPeers...); err != nil {
		s.Error("Failed to reset all btpeers: ", err)
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

		// Cleanup for power metrics service
		if tf.features.PowerEnabled {
			if _, err := dutConfig.PowerMetricsService.Cleanup(ctx, &empty.Empty{}); err != nil {
				s.Error("Clean up power metrics failed: ", err)
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
	for _, btsnoopCollector := range tf.btsnoopCollectors {
		if err := btsnoopCollector.Close(); err != nil {
			s.Error("Failed to close btsnoop log collectors: ", err)
		}
	}
	for _, btpeer := range tf.fv.BTPeers {
		btpeer.StopLogCollection(ctx)
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
	// Register the btpeer hosts.
	btpeerManager := GetBtpeerProvider()
	sshOptions := &ssh.Options{
		KeyDir:  s.DUT().KeyDir(),
		KeyFile: s.DUT().KeyFile(),
	}
	if btpeersVar, isSet := s.Var(fixtureVarBTPeers); isSet && btpeersVar != "" {
		btpeerHosts := strings.Split(btpeersVar, ",")
		if len(btpeerHosts) < requiredBTPeers {
			return errors.Errorf("fixture requires at least %d btpeers, but "+
				"only %d were provided in the %s tast var (%q)",
				requiredBTPeers, len(btpeerHosts),
				fixtureVarBTPeers, btpeersVar)
		}
		testing.ContextLogf(ctx, "Registering %d btpeer hosts from fixture var %q", len(btpeerHosts), fixtureVarBTPeers)
		if err := btpeerManager.RegisterBtpeerHosts(ctx, sshOptions, btpeerHosts...); err != nil {
			return errors.Wrapf(err, "failed to %d btpeer hosts from fixture var %q", len(btpeerHosts), fixtureVarBTPeers)
		}
	} else {
		// Imply btpeer hostnames based on DUT hostname.
		dutHostname := s.DUT().HostName()
		testing.ContextLogf(ctx, "Registering btpeer hosts based on dut hostname %q", dutHostname)
		if err := btpeerManager.RegisterBtpeerHostsByWificellDutHostname(ctx, sshOptions, dutHostname); err != nil {
			return errors.Wrapf(err, "failed to register btpeer hosts based on dut hostname %q", dutHostname)
		}
	}
	// Connect to the desired amount of hosts.
	testing.ContextLogf(ctx, "Connecting to %d btpeers", requiredBTPeers)
	btpeers, err := btpeerManager.ConnectAndReset(ctx, requiredBTPeers)
	if err != nil {
		return errors.Wrapf(err, "failed to connect to %d btpeers", requiredBTPeers)
	}
	testing.ContextLogf(ctx, "Successfully connected to %d btpeers", len(btpeers))
	// Start collecting logs on all the btpeers.
	testing.ContextLogf(ctx, "Starting log collection on %d btpeers", len(btpeers))
	for _, btpeer := range btpeers {
		if err := btpeer.StartLogCollection(s.FixtContext()); err != nil {
			return errors.Wrapf(err, "failed to start log collection on btpeer %s", btpeer)
		}
	}
	testing.ContextLogf(ctx, "Successfully set up %d btpeers", len(btpeers))
	tf.fv.BTPeers = btpeers
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
	for _, btpeer := range tf.fv.BTPeers {
		if err := btpeer.DumpLogs(ctx, logName); err != nil {
			return errors.Wrapf(err, "failed to dump logs for btpeer %q", btpeer.Hostname())
		}
	}
	for i, btmonLog := range tf.btsnoopCollectors {
		testing.ContextLogf(ctx, "Dump logs for btsnoop dut %d", i)
		dutName := fmt.Sprintf("dut%d", i)
		logDir := filepath.Join("btsnoop", dutName)
		if err := log.DumpCollectedLogsToFile(ctx, btmonLog, logDir, "btsnoop"); err != nil {
			return errors.Wrapf(err, "failed to dump collected btsnoop log from %s", dutName)
		}
	}
	return nil
}

// resetDutBluetoothState resets the bluetooth state of the DUT so that it is
// ready for tests.
func (tf *fixture) resetDutBluetoothState(ctx context.Context, dutConfig *DUTConfig, enableBluetooth bool) error {
	dutName := dutConfig.DUT.HostName()
	// Reset the state of the bluetooth adapter.
	testing.ContextLogf(ctx, "Resetting and setting bluetooth enabled to %t on DUT %s", enableBluetooth, dutName)
	if _, err := dutConfig.BluetoothService.Reset(ctx, &bts.ResetRequest{
		PowerOn: enableBluetooth,
	}); err != nil {
		return errors.Wrapf(err, "failed to reset and set bluetooth enabled to %t on DUT %s", enableBluetooth, dutName)
	}

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
	return nil
}
