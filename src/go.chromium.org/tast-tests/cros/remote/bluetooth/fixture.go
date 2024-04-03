// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/tape"
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

	// for power measurement
	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
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
	serviceDepDeviceSetupService   = "tast.cros.power.DeviceSetupService"
	serviceDepRecorderService      = "tast.cros.power.RecorderService"
)

// DUT D-Bus services.
const (
	dbusServiceBluetoothBluez        = "org.bluez"
	dbusServiceBluetoothFloss        = "org.chromium.bluetooth"
	dbusServiceBluetoothFlossManager = "org.chromium.bluetooth.Manager"
)

// Chrome features.
const (
	chromeFeatureOobeHidDetectionRevamp     = "OobeHidDetectionRevamp"
	chromeFeatureFastPair                   = "FastPair"
	chromeFeatureFastPairSavedDevices       = "FastPairSavedDevices"
	chromeFeatureFastPairHID                = "FastPairHID"
	chromeFeatureBluetoothDisconnectWarning = "BluetoothDisconnectWarning"

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

	// enableChromeUISetUpAndResetBuffer should be added to the setUpTimeout
	// and resetTimeout when fixtureFeatures.EnableChromeUI is true to give it
	// enough time to restart and log into chrome.
	// See the ChromeService implementation for more details.
	enableChromeUISetUpAndResetBuffer = 5 * time.Minute
)

type fixtureFeatures struct {
	// EnableChromeUI will ensure that chrome UI is enabled during the test if
	// true, or disabled if false.
	//
	// The enableChromeUISetUpAndResetBuffer must be added to the fixture SetUp when this is
	// true to give it enough time.
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

	// PowerEnabled allows for power measurement
	PowerEnabled bool
}

type onDutCleanup func(ctx context.Context, dutConfig *DUTConfig)

// DUTConfig groups DUT-specific fixture configs and utils.
type DUTConfig struct {
	// Cleanup functions to call when SetUp fails or during TearDown to free
	// resources. Not allowed to fail; errors should be logged.
	// Processed in reverse order (FILO), list cleared when processed.
	onDutCleanupStack []onDutCleanup

	btsnoopCollector             *log.BtsnoopCollector
	bluetoothServicesDBusMonitor *log.DBusMonitorCollector
	uiEnabled                    bool

	// DUT is the connection to the DUT.
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

	// PowerDeviceSetupService is a client of the DeviceSetupService that set-up
	// the device for measuring power consumption.
	PowerDeviceSetupService power.DeviceSetupServiceClient

	// PowerRecorderService is a client of the RecorderService that measures the
	// power consumption.
	PowerRecorderService power.RecorderServiceClient

	startChromeOption *ui.NewRequest
}

func (c *DUTConfig) addCleanupStep(f onDutCleanup) {
	c.onDutCleanupStack = append(c.onDutCleanupStack, f)
}

func (c *DUTConfig) cleanup(ctx context.Context) {
	testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] cleanup :: DUT %q :: START", c.DUT.HostName())
	defer testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] cleanup :: DUT %q :: END", c.DUT.HostName())
	for i := len(c.onDutCleanupStack) - 1; i >= 0; i-- {
		c.onDutCleanupStack[i](ctx, c)
	}
	c.onDutCleanupStack = nil
}

func (c *DUTConfig) dumpLogs(ctx context.Context, logName string) error {
	if c.bluetoothServicesDBusMonitor != nil {
		if err := c.dumpBluetoothServiceDBusLogs(ctx, logName); err != nil {
			return errors.Wrapf(err, "failed to dump bluetooth service dbus logs from DUT %q", c.DUT.HostName())
		}
	}
	if c.btsnoopCollector != nil {
		if err := c.dumpBtsnoopLogs(ctx, logName); err != nil {
			return errors.Wrapf(err, "failed to dump btsnoop logs from DUT %q", c.DUT.HostName())
		}
	}
	return nil
}

func (c *DUTConfig) dumpBluetoothServiceDBusLogs(ctx context.Context, logName string) error {
	if c.bluetoothServicesDBusMonitor == nil {
		return errors.New("bluetoothServicesDBusMonitor is nil")
	}
	testing.ContextLogf(ctx, "Dumping collected bluetooth service dbus monitor logs from DUT %q for %q", c.DUT.HostName(), logName)
	cleanDutName := regexp.MustCompile(`[\W_]+`).ReplaceAllString(c.DUT.HostName(), "_")
	logDir := filepath.Join("dbus_monitor_bluetooth", cleanDutName)
	if err := log.DumpCollectedLogsToFile(ctx, c.bluetoothServicesDBusMonitor, logDir, logName); err != nil {
		return errors.Wrapf(err, "failed to dump collected dbus-monitor messages from DUT %q", c.DUT.HostName())
	}
	return nil
}

func (c *DUTConfig) dumpBtsnoopLogs(ctx context.Context, logName string) error {
	if c.btsnoopCollector == nil {
		return errors.New("btsnoopCollector is nil")
	}
	testing.ContextLogf(ctx, "Dumping collected btsnoop logs from DUT %q for %q", c.DUT.HostName(), logName)
	cleanDutName := regexp.MustCompile(`[\W_]+`).ReplaceAllString(c.DUT.HostName(), "_")
	logDir := filepath.Join("btsnoop", cleanDutName)
	if err := log.DumpCollectedLogsToFile(ctx, c.btsnoopCollector, logDir, logName); err != nil {
		return errors.Wrapf(err, "failed to dump collected btsnoop log from DUT %q", c.DUT.HostName())
	}
	return nil
}

// DUTReLogin checks that we are currently logged in, and afterwards will log
// the user out then log the user back in.
// An error will be thrown if the DUT hasn't logged in yet.
func (c *DUTConfig) DUTReLogin(ctx context.Context) error {
	if !c.uiEnabled {
		return errors.New("DUT does not have the UI enabled")
	}

	if c.startChromeOption.LoginMode == ui.LoginMode_LOGIN_MODE_NO_LOGIN {
		return errors.New("DUT is not logged in")
	}

	if _, err := c.ChromeService.Close(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrap(err, "failed to releases the chrome session from the dut")
	}

	c.startChromeOption.KeepState = true
	_, err := c.ChromeService.New(ctx, c.startChromeOption)
	return err
}

func newDUTConfig(ctx context.Context, dut *dut.DUT, RPCHint *testing.RPCHint) (*DUTConfig, error) {
	rpcClient, err := rpc.Dial(ctx, dut, RPCHint)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to Connect to the local gRPC service on DUT %s", dut.HostName())
	}
	dutConfig := &DUTConfig{
		DUT:                     dut,
		DUTRPCClient:            rpcClient,
		BluetoothService:        bts.NewBluetoothServiceClient(rpcClient.Conn),
		BluetoothUIService:      bts.NewBluetoothUIServiceClient(rpcClient.Conn),
		ChromeService:           ui.NewChromeServiceClient(rpcClient.Conn),
		UpstartService:          platform.NewUpstartServiceClient(rpcClient.Conn),
		AudioService:            ui.NewAudioServiceClient(rpcClient.Conn),
		QuickSettingsService:    qs.NewQuickSettingsServiceClient(rpcClient.Conn),
		PowerDeviceSetupService: power.NewDeviceSetupServiceClient(rpcClient.Conn),
		PowerRecorderService:    power.NewRecorderServiceClient(rpcClient.Conn),
	}
	dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
		if err := dutConfig.DUTRPCClient.Close(ctx); err != nil {
			testing.ContextLog(ctx, "WARNING: Failed to close DUTRPCClient: ", err)
		}
	})
	return dutConfig, nil
}

// FixtValue is the value of the test fixture accessible within a test. All
// variables are configured in fixture.SetUp so that tests can use them without
// any further configuration.
type FixtValue struct {
	// Internal fixture vars set during SetUp.
	chromeUsername               string
	chromePassword               string
	signinProfileTestExtensionID string

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

	// PowerDeviceSetupService is a client of the DeviceSetupService that set-up
	// the device for measuring power consumption.
	PowerDeviceSetupService power.DeviceSetupServiceClient

	// PowerRecorderService is a client of the RecorderService that measures the
	// power consumption.
	PowerRecorderService power.RecorderServiceClient

	// PrimaryDutName is the host name of the primary DUT.
	PrimaryDutName string
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

// StartPowerRecording to cooldown the device and start a recording for power metrics.
func (fv *FixtValue) StartPowerRecording(ctx context.Context) error {
	testing.ContextLog(ctx, "Cooling down device")
	if _, err := fv.PowerRecorderService.Cooldown(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to cooldown device")
	}
	testing.ContextLog(ctx, "Start recording power metrics")
	if _, err := fv.PowerRecorderService.Start(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start recording power metrics")
	}
	return nil
}

// StopPowerRecording to stop the existing power recording.
func (fv *FixtValue) StopPowerRecording(ctx context.Context, uploadTestName string) (*perf.Values, error) {
	testing.ContextLog(ctx, "Stop recording power metrics")

	rRes, err := fv.PowerRecorderService.Stop(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to stop recording metrics")
	}
	perfVals := perf.NewValuesFromProto(rRes.GetPerfMetrics())

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get OutDir")
	}

	var logDir string
	for i, c := range fv.DUTConfigs {
		if c.DUT.HostName() == fv.PrimaryDutName {
			logDir = filepath.Join(outDir, fmt.Sprintf("power_dut%d_%s", i, uploadTestName))
			if err := os.MkdirAll(logDir, 0755); err != nil {
				return nil, errors.Wrap(err, "failed to create log directory")
			}
			break
		}
	}

	if _, err := cp.CreateSaveUploadPowerLog(
		ctx,
		logDir,
		uploadTestName,
		"",
		perfVals,
		nil,
		rRes.GetDeviceInfo(),
		rRes.GetOneTimeMetrics(),
	); err != nil {
		return nil, errors.Wrap(err, "failed to save and upload power log")
	}

	if err := perfVals.Save(logDir); err != nil {
		return nil, errors.Wrap(err, "failed to save perf data for crosbolt")
	}

	return perfVals, nil
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
	btStack         bts.BluetoothStackType

	// Stateful vars which are initialized during SetUp.
	fv *FixtValue
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
	// Determine desired bluetooth stack for DUTs.
	if tf.features.FlossEnabled {
		tf.btStack = bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS
	} else {
		tf.btStack = bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ
	}
	if tf.features.EnableChromeUI {
		if tf.btStack == bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_FLOSS {
			tf.features.EnableFeatures = append(tf.features.EnableFeatures, chromeFeatureFloss)
			tf.features.DisableFeatures = append(tf.features.DisableFeatures, chromeFeatureFlossIsAvailabilityCheckNeeded)
		} else {
			tf.features.DisableFeatures = append(tf.features.DisableFeatures, chromeFeatureFloss)
		}
	}
	return tf
}

// SetUp preforms fixture setup actions. All fixtureFeatures are configured.
//
// This is necessary to implement testing.FixtureImpl.
func (tf *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	s.Log("[BLUETOOTH_FIXTURE] SetUp :: START")
	defer s.Log("[BLUETOOTH_FIXTURE] SetUp :: END")

	// Clear stateful fixture properties on setup and on setup failure.
	tf.fv = &FixtValue{}
	defer func() {
		if s.HasError() {
			tf.fv = nil
		}
	}()

	// Parse OOBE fixture var.
	if tf.features.EnableHidScreenOnOobe {
		signinProfileTestExtensionID, ok := s.Var(fixtureVarSigninKey)
		if !ok {
			s.Fatal("Failed to get sign-in key variable required for OOBE tests")
		}
		tf.fv.signinProfileTestExtensionID = signinProfileTestExtensionID
	}

	// Connect to btpeers and reset them to a fresh state.
	if err := tf.setUpBTPeers(ctx, s, tf.features.BTPeerCount); err != nil {
		s.Fatal("Failed to set up btpeers: ", err)
	}

	// Cleanup duts if anything goes wrong during their setup process.
	cleanupCtx := ctx
	defer func(ctx context.Context) {
		if s.HasError() {
			tf.cleanupAllDuts(ctx)
		}
	}(cleanupCtx)
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Configure primary DUT.
	primaryDUTConfig, err := newDUTConfig(s.FixtContext(), s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to configure primary DUT: ", err)
	}
	tf.fv.DUTConfigs = []*DUTConfig{primaryDUTConfig}

	// Save shortcuts for primary DUT configs for ease of use in most tests.
	tf.fv.PrimaryDutName = primaryDUTConfig.DUT.HostName()
	tf.fv.DUTRPCClient = primaryDUTConfig.DUTRPCClient
	tf.fv.BluetoothUIService = primaryDUTConfig.BluetoothUIService
	tf.fv.BluetoothService = primaryDUTConfig.BluetoothService
	tf.fv.ChromeService = primaryDUTConfig.ChromeService
	tf.fv.UpstartService = primaryDUTConfig.UpstartService
	tf.fv.AudioService = primaryDUTConfig.AudioService
	tf.fv.QuickSettingsService = primaryDUTConfig.QuickSettingsService
	tf.fv.PowerRecorderService = primaryDUTConfig.PowerRecorderService
	tf.fv.PowerDeviceSetupService = primaryDUTConfig.PowerDeviceSetupService

	// Configure companion DUT.
	if tf.features.RequireCompanionDUT {
		var companionDUTRoles []string
		for role := range s.CompanionDUTs() {
			companionDUTRoles = append(companionDUTRoles, role)
		}
		if len(companionDUTRoles) != 1 {
			s.Fatalf(
				"Failed to get companion DUT: expected 1 companion DUT, found %d: %s",
				len(companionDUTRoles),
				strings.Join(companionDUTRoles, ", "),
			)
		}
		companionDUTRole := companionDUTRoles[0]
		companionDUT := s.CompanionDUT(companionDUTRole)
		if companionDUT == nil {
			s.Fatalf("Failed to get companion DUT %q", companionDUTRole)
		}
		companionDUTConfig, err := newDUTConfig(s.FixtContext(), companionDUT, s.RPCHint())
		if err != nil {
			s.Fatalf("Failed to configure companion DUT %q: %v", companionDUTRole, err)
		}
		tf.fv.DUTConfigs = append(tf.fv.DUTConfigs, companionDUTConfig)
	}

	// Resolve credentials to use for all DUTs when UI is enabled.
	if tf.features.EnableChromeUI {
		tf.resolveChromeCredentials(ctx, s)
	}

	// Perform per-DUT setup actions.
	for dutIndex, dutConfig := range tf.fv.DUTConfigs {
		if err := tf.setUpDut(ctx, dutConfig); err != nil {
			s.Fatalf("Failed to set up DUT[%d] %q: %v", dutIndex, dutConfig.DUT.HostName(), err)
		}
	}

	// Save collected logs collected thus far from setup process actions.
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
	testing.ContextLog(ctx, "[BLUETOOTH_FIXTURE] Reset :: START")
	defer testing.ContextLog(ctx, "[BLUETOOTH_FIXTURE] Reset :: END")
	if err := GetBtpeerProvider().Reset(ctx, tf.fv.BTPeers...); err != nil {
		return errors.Wrap(err, "failed to reset all btpeers")
	}
	for _, dutConfig := range tf.fv.DUTConfigs {
		if _, err := dutConfig.BluetoothService.Reset(ctx, &bts.ResetRequest{
			PowerOn: true,
		}); err != nil {
			return errors.Wrap(err, "failed to reset DUT bluetooth stack")
		}
		if tf.features.EnableChromeUI {
			if err := tf.ResetChromeUI(ctx, dutConfig, true); err != nil {
				return errors.Wrapf(err, "failed to reset Chrome UI of DUT %s", dutConfig.DUT.HostName())
			}
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
	s.Log("[BLUETOOTH_FIXTURE] PostTest :: START")
	defer s.Log("[BLUETOOTH_FIXTURE] PostTest :: END")
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
	s.Log("[BLUETOOTH_FIXTURE] TearDown :: START")
	defer s.Log("[BLUETOOTH_FIXTURE] TearDown :: END")
	// Reset btpeers.
	if err := GetBtpeerProvider().Reset(ctx, tf.fv.BTPeers...); err != nil {
		s.Error("Failed to reset all btpeers: ", err)
	}

	// Tear down each DUT.
	tf.cleanupAllDuts(ctx)

	// Dump and close log collectors.
	if err := tf.dumpAllCollectedLogs(ctx, "TearDown"); err != nil {
		s.Error("Failed to dump logs: ", err)
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

	// Clear stateful fixture properties.
	tf.fv = nil
}

func (tf *fixture) setUpBTPeers(ctx context.Context, s *testing.FixtState, requiredBTPeers int) error {
	s.Log("[BLUETOOTH_FIXTURE] setUpBTPeers :: START")
	defer s.Log("[BLUETOOTH_FIXTURE] setUpBTPeers :: END")
	ctx, st := timing.Start(ctx, fmt.Sprintf("setUpBTPeers_%d", requiredBTPeers))
	defer st.End()
	if requiredBTPeers <= 0 {
		return nil
	}
	// Register the btpeer hosts.
	btpeerProvider := GetBtpeerProvider()
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
		btpeerProvider.RegisterBtpeerHosts(ctx, sshOptions, btpeerHosts...)
	} else {
		// Imply btpeer hostnames based on DUT hostname.
		dutHostname := s.DUT().HostName()
		testing.ContextLogf(ctx, "Registering btpeer hosts based on dut hostname %q", dutHostname)
		if err := btpeerProvider.RegisterBtpeerHostsByWificellDutHostname(ctx, sshOptions, dutHostname); err != nil {
			return errors.Wrapf(err, "failed to register btpeer hosts based on dut hostname %q", dutHostname)
		}
	}
	// Connect to the desired amount of hosts.
	testing.ContextLogf(ctx, "Connecting to %d btpeers", requiredBTPeers)
	btpeers, err := btpeerProvider.ConnectAndReset(ctx, requiredBTPeers)
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

func (tf *fixture) setUpDut(ctx context.Context, dutConfig *DUTConfig) error {
	testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] setUpDut :: DUT %q :: START", dutConfig.DUT.HostName())
	defer testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] setUpDut :: DUT %q :: END", dutConfig.DUT.HostName())

	// Stop the Chrome UI if it was running from a prior test, but do not close
	// the UI services as those may not have been configured yet.
	if err := tf.stopChromeUI(ctx, dutConfig, true); err != nil {
		return errors.Wrap(err, "failed to stop Chrome UI")
	}

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
		return errors.Wrap(err, "failed to start dbus-monitor listening to bluez and floss service messages")
	}
	dutConfig.bluetoothServicesDBusMonitor = bluetoothServicesDBusMonitor
	dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
		if err := dutConfig.dumpBluetoothServiceDBusLogs(ctx, "cleanup"); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to dump bluetooth service dbus logs from DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
		if err := dutConfig.bluetoothServicesDBusMonitor.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close bluetooth service dbus log collector for DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
		dutConfig.bluetoothServicesDBusMonitor = nil
	})

	btsnoopCollector, err := log.StartBtsnoopCollector(ctx, dutConfig.DUT.Conn())
	if err != nil {
		return errors.Wrap(err, "failed to start btsnoop log")
	}
	dutConfig.btsnoopCollector = btsnoopCollector
	dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
		if err := dutConfig.dumpBtsnoopLogs(ctx, "cleanup"); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to dump btsnoop logs from DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
		if err := dutConfig.btsnoopCollector.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close btsnoop log collector for DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
		dutConfig.btsnoopCollector = nil
	})

	// Configure and enable desired DUT bluetooth stack.
	testing.ContextLogf(ctx, "=== Configuring DUT to use bluetooth stack %q ===", tf.btStack)
	if _, err := dutConfig.BluetoothService.SetBluetoothStack(ctx, &bts.SetBluetoothStackRequest{
		StackType: tf.btStack,
	}); err != nil {
		return errors.Wrapf(err, "failed to set DUT bluetooth stack as %q", tf.btStack)
	}
	if _, err := dutConfig.BluetoothService.Enable(ctx, &emptypb.Empty{}); err != nil {
		return errors.Wrapf(err, "failed to enable bluetooth on DUT with stack %q", tf.btStack)
	}
	if _, err := dutConfig.BluetoothService.Reset(ctx, &bts.ResetRequest{
		PowerOn: true,
	}); err != nil {
		return errors.Wrap(err, "failed to reset DUT bluetooth stack")
	}
	if tf.btStack == bts.BluetoothStackType_BLUETOOTH_STACK_TYPE_BLUEZ {
		if _, err = tf.fv.BluetoothService.SetDebugLogLevels(ctx, &bts.SetDebugLogLevelsRequest{Level: 1}); err != nil {
			return errors.Wrap(err, "failed to set bluetooth debug log levels")
		}
	}
	dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
		if _, err := dutConfig.BluetoothService.Reset(ctx, &bts.ResetRequest{
			PowerOn: true,
		}); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to reset bluetooth stack of DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
		testing.ContextLog(ctx, "Disabling bluetooth on DUT")
		if _, err := dutConfig.BluetoothService.Disable(ctx, &emptypb.Empty{}); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to disable bluetooth stack on DUT %q: %v", dutConfig.DUT.HostName(), err)
		}
	})

	if err := tf.startChromeUI(ctx, dutConfig); err != nil {
		return errors.Wrap(err, "failed to start Chrome UI")
	}

	// Set up power test
	if tf.features.PowerEnabled {
		testing.ContextLog(ctx, "=== Configure Power Measurement ===")
		setupRequest := power.DeviceSetupRequest{
			Ui:                 power.UIMode_DISABLE_UI,
			ScreenBrightness:   power.ScreenMode_ZERO_SCREEN_BRIGHTNESS,
			KeyboardBrightness: power.KeyboardMode_ZERO_KEYBOARD_BRIGHTNESS,
			Wifi:               power.WifiMode_DISABLE_WIFI,
			Bluetooth:          power.BluetoothMode_DO_NOT_CHANGE_BLUETOOTH,
		}
		if tf.features.EnableChromeUI {
			testing.ContextLog(ctx, "Allow UI for power measurement")
			setupRequest.Ui = power.UIMode_DO_NOT_CHANGE_UI
		}
		if _, err := dutConfig.PowerDeviceSetupService.Setup(ctx, &setupRequest); err != nil {
			return errors.Wrap(err, "failed to set up device for power measurement")
		}
		dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
			if _, err := dutConfig.PowerDeviceSetupService.Cleanup(ctx, &empty.Empty{}); err != nil {
				testing.ContextLog(ctx, "WARNING: Failed to call PowerDeviceSetupService.Cleanup: ", err)
			}
		})
		recorderRequest := power.RecorderRequest{
			IntervalSec: DefaultBTPowerIntervalSecond,
		}
		if _, err := dutConfig.PowerRecorderService.Create(ctx, &recorderRequest); err != nil {
			return errors.Wrap(err, "failed to set up recorder for power measurement")
		}
		dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
			if _, err := dutConfig.PowerRecorderService.Close(ctx, &empty.Empty{}); err != nil {
				testing.ContextLogf(ctx, "WARNING: Failed to call PowerRecorderService.Close on DUT %q: %v", dutConfig.DUT.HostName(), err)
			}
		})
	}

	return nil
}

func (tf *fixture) cleanupAllDuts(ctx context.Context) {
	testing.ContextLog(ctx, "[BLUETOOTH_FIXTURE] cleanupAllDuts :: START")
	defer testing.ContextLog(ctx, "[BLUETOOTH_FIXTURE] cleanupAllDuts :: END")
	for _, dutConfig := range tf.fv.DUTConfigs {
		dutConfig.cleanup(ctx)
	}
}

func (tf *fixture) resolveChromeCredentials(ctx context.Context, s *testing.FixtState) {
	s.Log("[BLUETOOTH_FIXTURE] resolveChromeCredentials :: START")
	defer s.Log("[BLUETOOTH_FIXTURE] resolveChromeCredentials :: END")
	if tf.features.RequireFastPairUserVars {
		// Fast Pair tests require GAIA credentials to be provided, which can be
		// passed via CLI or will use the default credentials.
		tf.fv.chromeUsername = s.RequiredVar(fixtureVarFastPairChromeUsername)
		tf.fv.chromePassword = s.RequiredVar(fixtureVarFastPairChromePassword)
		s.Logf("Using Fast Pair test chrome user credentials for user %q", tf.fv.chromeUsername)
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
		tf.fv.chromeUsername = tf.fv.tapeAccount.Username
		tf.fv.chromePassword = tf.fv.tapeAccount.Password
		s.Logf("Using Fast Pair OTA chrome user credentials leased with Tape for user %q", tf.fv.chromeUsername)
	} else {
		// By default, use the default username/password used for Fake login.
		tf.fv.chromeUsername = defaultChromeUsername
		tf.fv.chromePassword = defaultChromePassword
		s.Log("Using default fake chrome user credentials")
	}
}

func (tf *fixture) stopChromeUI(ctx context.Context, dutConfig *DUTConfig, stopUIJob bool) error {
	testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] stopChromeUI :: DUT %q :: START", dutConfig.DUT.HostName())
	defer testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] stopChromeUI :: DUT %q :: END", dutConfig.DUT.HostName())
	if dutConfig.uiEnabled {
		testing.ContextLog(ctx, "=== Closing ChromeService ===")
		if _, err := dutConfig.ChromeService.Close(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to call ChromeService.Close")
		}
		dutConfig.uiEnabled = false
	}
	if stopUIJob {
		testing.ContextLog(ctx, "=== Stopping Chrome UI job ===")
		if _, err := dutConfig.UpstartService.StopJob(ctx, &platform.StopJobRequest{
			JobName: "ui",
		}); err != nil {
			return errors.Wrapf(err, "failed to stop Chrome UI job on DUT %q", dutConfig.DUT.HostName())
		}
		dutConfig.uiEnabled = false
	}
	return nil
}

// startChromeUI will start the UI service by logging into chrome either with
// the ChromeService if EnableChromeUI feature is set or the AudioService if
// the EnableAudioUI feature is set. No-op if neither are set.
func (tf *fixture) startChromeUI(ctx context.Context, dutConfig *DUTConfig) error {
	testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] startChromeUI :: DUT %q :: START", dutConfig.DUT.HostName())
	defer testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] startChromeUI :: DUT %q :: END", dutConfig.DUT.HostName())
	if tf.features.EnableChromeUI {
		testing.ContextLog(ctx, "=== Starting Chrome UI with ChromeService ===")
		var extraArgs []string
		if tf.fastPairEnabled {
			extraArgs = fixtureVarFastPairExtraArgs
		}
		dutConfig.startChromeOption = &ui.NewRequest{
			LoginMode:       tf.features.LoginMode,
			EnableFeatures:  tf.features.EnableFeatures,
			DisableFeatures: tf.features.DisableFeatures,
			Credentials: &ui.NewRequest_Credentials{
				Username: tf.fv.chromeUsername,
				Password: tf.fv.chromePassword,
			},
			EnableHidScreenOnOobe:        tf.features.EnableHidScreenOnOobe,
			SigninProfileTestExtensionId: tf.fv.signinProfileTestExtensionID,
			ExtraArgs:                    extraArgs,
		}

		if _, err := dutConfig.ChromeService.New(ctx, dutConfig.startChromeOption); err != nil {
			return errors.Wrap(err, "failed to start Chrome UI with ChromeService")
		}
		dutConfig.uiEnabled = true
		dutConfig.addCleanupStep(func(ctx context.Context, dutConfig *DUTConfig) {
			if !dutConfig.uiEnabled {
				return
			}
			if tf.fastPairEnabled {
				if _, err := dutConfig.BluetoothUIService.RemoveAllSavedDevices(ctx, &emptypb.Empty{}); err != nil {
					testing.ContextLogf(ctx, "WARNING: Failed to remove all saved bluetooth devices via UI on DUT %q: %v", dutConfig.DUT.HostName(), err)
				}
			}
			if _, err := dutConfig.ChromeService.Close(ctx, &emptypb.Empty{}); err != nil {
				testing.ContextLogf(ctx, "WARNING: Failed to close Chrome UI with ChromeService for DUT %q: %v", dutConfig.DUT.HostName(), err)
			}
			dutConfig.uiEnabled = false
		})
	} else {
		testing.ContextLog(ctx, "Skipping start of Chrome UI as no Chrome UI fixture features are enabled for this fixture")
	}
	return nil
}

// ResetChromeUI will log out of chrome, optionally restart the UI job, then log
// back into chrome. Only supported if UI features are enabled for the fixture.
func (tf *fixture) ResetChromeUI(ctx context.Context, dutConfig *DUTConfig, restartUIJob bool) error {
	testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] ResetChromeUI :: DUT %q :: START", dutConfig.DUT.HostName())
	defer testing.ContextLogf(ctx, "[BLUETOOTH_FIXTURE] ResetChromeUI :: DUT %q :: END", dutConfig.DUT.HostName())
	if !tf.features.EnableChromeUI {
		return errors.New("the Chrome UI is not enabled for this fixture")
	}
	if err := tf.stopChromeUI(ctx, dutConfig, restartUIJob); err != nil {
		return errors.Wrap(err, "failed to log out of chrome")
	}
	if err := tf.startChromeUI(ctx, dutConfig); err != nil {
		return errors.Wrap(err, "failed to log back into chrome")
	}
	if tf.fastPairEnabled {
		testing.ContextLog(ctx, "Removing all saved bluetooth devices via UI")
		if _, err := dutConfig.BluetoothUIService.RemoveAllSavedDevices(ctx, &emptypb.Empty{}); err != nil {
			return errors.Wrap(err, "failed to remove all saved bluetooth devices via UI")
		}
	}
	return nil
}

func (tf *fixture) dumpAllCollectedLogs(ctx context.Context, logName string) error {
	ctx, st := timing.Start(ctx, "dumpAllCollectedLogs")
	defer st.End()
	// Dump DUT logs.
	for _, dutConfig := range tf.fv.DUTConfigs {
		if err := dutConfig.dumpLogs(ctx, logName); err != nil {
			return errors.Wrapf(err, "failed to dump logs for DUT %q", dutConfig.DUT.HostName())
		}
	}
	// Dump btpeer logs.
	for _, btpeer := range tf.fv.BTPeers {
		if err := btpeer.DumpLogs(ctx, logName); err != nil {
			return errors.Wrapf(err, "failed to dump logs for btpeer %q", btpeer.Hostname())
		}
	}
	return nil
}
