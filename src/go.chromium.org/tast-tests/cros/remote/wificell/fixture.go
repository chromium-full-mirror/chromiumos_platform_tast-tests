// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/wificell/wifiutil"
	"go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// Timeout for methods of Tast fixture.
const (
	// Give long enough timeout for SetUp() and TearDown() as they might need
	// to reboot a broken DUT. SetUp() and Reset() have additional time allotted
	// to reboot routers as well.
	setUpTimeout         = 17 * time.Minute
	tearDownTimeout      = 5 * time.Minute
	resetTimeout         = 11 * time.Minute
	preTestTimeout       = 30 * time.Second
	postTestTimeout      = 30 * time.Second
	enrollmentRunTimeout = 4 * time.Minute
	enrollRetry          = 3
	idlePowerSleepTime   = 50 * time.Second
	powerSetUpTimeout    = 2*idlePowerSleepTime + setUpTimeout
)

// Fixture vars.
const (
	// fixtureVarRouter is the fixture var for setting
	// TFOptions.PrimaryRouterTargets with a single router when fixtures do
	// not have the TFFeaturesRouters feature.
	fixtureVarRouter = "router"

	// fixtureVarRoutersMultiple is the fixture var for setting
	// TFOptions.PrimaryRouterTargets with a multiple routers when fixtures
	// have the TFFeaturesRouters feature.
	//
	// Expects comma-separated hostnames.
	//
	// If this is left unset but fixtureVarRouter and fixtureVarPcap are set,
	// they will be treated as if they were entries of this var in that order
	// when this var is needed.
	fixtureVarRoutersMultiple = "routers"

	// fixtureVarPcap is the fixture var for setting TFOptions.PcapRouterTarget
	// for all fixtures.
	fixtureVarPcap = "pcap"

	// fixtureVarAttenuator is the fixture var for setting
	// TFOptions.AttenuatorTarget for fixtures that have the TFFeaturesAttenuator
	// feature.
	fixtureVarAttenuator = "attenuator"

	// fixtureVarEnableRouterReboot is the fixture var for setting
	// TFOptions.EnableRouterReboot for all fixtures.
	fixtureVarEnableRouterReboot = "wificell.EnableRouterReboot"
)

func init() {
	// Most of the fixture variants are based on some default values. Let's generate common data in a loop,
	// then adjust non-standard values and register fixtures in another loop.

	// ID and Description is the most individual part of the fixture, prepare their map first.
	params := make(map[TFFeatures]string)
	params[TFFeaturesNone] = "Default wificell setup with router and pcap object. Note that pcap and router can point to the same Access Point. Also, unlike wificellFixtWithCapture, the fixture won't spawn Capturer. Users may spawn Capturer with customized configuration when needed"
	params[TFFeaturesCapture] = "Wificell setup with Capturer on pcap for each configured AP"
	params[TFFeaturesCapture|TFFeaturesRouterAsCapture] = "Wificell setup with default capturer on router instead of pcap"
	params[TFFeaturesRouters] = "Wificell setup with multiple routers"
	params[TFFeaturesRouters|TFFeaturesAttenuator] = "WiFi roaming setup with multiple routers and attenuators"
	params[TFFeaturesEnroll] = "Wificell setup with router and pcap object and chrome enrolled"
	params[TFFeaturesCompanionDUT] = "Wificell setup with companion Chromebook DUT"
	params[TFFeaturesCompanionDUT|TFFeaturesSelfManagedAP] = "Wificell setup with companion Chromebook DUT and a self managed AP"
	params[TFFeaturesCompanionDUT|TFFeaturesCapture] = "Wificell setup with companion Chromebook DUT and packet capture from the pcap device"
	params[TFFeaturesCompanionDUT|TFFeaturesSelfManagedAP|TFFeaturesCapture] = "Wificell setup with companion Chromebook DUT and a self managed AP and packet capture from the pcap device"
	params[TFFeaturesPower] = "Default wificell setup with power diagnostics"
	params[TFFeaturesCellular] = "Wificell setup on a cellular capable device"
	params[TFFeaturesCompanionDUT|TFFeaturesCellular] = "Wificell setup on a cellular capable device with companion chromebook DUT"
	params[TFFeaturesCompanionDUT|TFFeaturesSelfManagedAP|TFFeaturesCellular] = "Wificell setup on a cellular capable device with companion chromebook DUT and self managed AP"
	params[TFFeaturesWithUI] = "Wificell setup with the UI"

	fixtures := make(map[TFFeatures]*testing.Fixture)
	for f, desc := range params {
		fixtures[f] = &testing.Fixture{
			Name: f.String(),
			Desc: desc,
			// Default fixture configuration.
			Contacts: []string{
				"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
			},
			Impl:            newTastFixture(f),
			SetUpTimeout:    setUpTimeout,
			ResetTimeout:    resetTimeout,
			PreTestTimeout:  preTestTimeout,
			PostTestTimeout: postTestTimeout,
			TearDownTimeout: tearDownTimeout,
			ServiceDeps:     []string{ShillServiceName, BluetoothServiceName},
			Vars: []string{
				fixtureVarRouter,
				fixtureVarPcap,
				fixtureVarEnableRouterReboot,
			},
		}

		// Typical fixture extensions.
		if f&TFFeaturesRouters != 0 {
			fixtures[f].Vars = append(fixtures[f].Vars, fixtureVarRoutersMultiple)
		}
		if f&TFFeaturesAttenuator != 0 {
			fixtures[f].Vars = append(fixtures[f].Vars, fixtureVarAttenuator)
		}
		if f&TFFeaturesPower != 0 {
			fixtures[f].ServiceDeps = append(fixtures[f].ServiceDeps, PowerSetupServiceName)
			fixtures[f].ServiceDeps = append(fixtures[f].ServiceDeps, PowerRecorderServiceName)
		}
		if f&TFFeaturesCellular != 0 {
			fixtures[f].ServiceDeps = append(fixtures[f].ServiceDeps, CellularServiceName)
		}
	}

	// Non-default values.
	fixtures[TFFeaturesEnroll].ServiceDeps = append(
		fixtures[TFFeaturesEnroll].ServiceDeps,
		"tast.cros.hwsec.OwnershipService",
		"tast.cros.policy.PolicyService",
		"tast.cros.graphics.ScreenshotService",
	)
	fixtures[TFFeaturesEnroll].SetUpTimeout = 10 * time.Minute
	fixtures[TFFeaturesEnroll].TearDownTimeout = 8 * time.Minute

	// Register prepared fixtures.
	for _, f := range fixtures {
		testing.AddFixture(f)
	}
}

// TFFeatures is an enum type for extra features needed for Tast fixture.
// Note that features can be combined using bitwise OR, e.g. TFFeaturesCapture | TFFeaturesRouters.
type TFFeatures uint16

const (
	// TFFeaturesNone represents a default value.
	TFFeaturesNone TFFeatures = 0
	// TFFeaturesCapture is a feature that spawns packet capturer in TestFixture.
	TFFeaturesCapture = 1 << iota
	// TFFeaturesRouters allows to configure more than one router.
	TFFeaturesRouters
	// TFFeaturesAttenuator feature facilitates attenuator handling.
	TFFeaturesAttenuator
	// TFFeaturesRouterAsCapture configures the router as a capturer as well.
	TFFeaturesRouterAsCapture
	// TFFeaturesEnroll enrolls Chrome.
	TFFeaturesEnroll
	// TFFeaturesCompanionDUT is a feature that spawns companion DUT in TestFixture.
	TFFeaturesCompanionDUT
	// TFFeaturesSelfManagedAP is a feature that uses a companion DUT and a self managed AP.
	TFFeaturesSelfManagedAP
	// TFFeaturesPower is a feature that enables power measurements.
	TFFeaturesPower
	// TFFeaturesCellular set up cellular shill service on the DUT.
	TFFeaturesCellular
	// TFFeaturesWithUI ensures the UI is started as part of the fixture setup.
	TFFeaturesWithUI
)

// String returns name component corresponding to enum value(s).
func (enum TFFeatures) String() string {
	if enum == 0 {
		return "wificellFixt"
	}
	ret := []string{"wificellFixt"}
	if enum&TFFeaturesCapture != 0 {
		ret = append(ret, "Capture")
		// Punch out the bit to check for weird values later.
		enum ^= TFFeaturesCapture
	}
	if enum&TFFeaturesRouters != 0 {
		ret = append(ret, "Routers")
		enum ^= TFFeaturesRouters
	}
	if enum&TFFeaturesAttenuator != 0 {
		ret = append(ret, "Attenuator")
		enum ^= TFFeaturesAttenuator
	}
	if enum&TFFeaturesRouterAsCapture != 0 {
		ret = append(ret, "RouterAsPcap")
		enum ^= TFFeaturesRouterAsCapture
	}
	if enum&TFFeaturesEnroll != 0 {
		ret = append(ret, "Enrolled")
		enum ^= TFFeaturesEnroll
	}
	if enum&TFFeaturesCompanionDUT != 0 {
		ret = append(ret, "CompanionDut")
		enum ^= TFFeaturesCompanionDUT
	}
	if enum&TFFeaturesSelfManagedAP != 0 {
		ret = append(ret, "SelfManagedAP")
		enum ^= TFFeaturesSelfManagedAP
	}
	if enum&TFFeaturesPower != 0 {
		ret = append(ret, "Power")
		enum ^= TFFeaturesPower
	}
	if enum&TFFeaturesCellular != 0 {
		ret = append(ret, "Cellular")
		enum ^= TFFeaturesCellular
	}
	if enum&TFFeaturesWithUI != 0 {
		ret = append(ret, "WithUI")
		enum ^= TFFeaturesWithUI
	}
	// Catch weird cases. Like when somebody extends enum, but forgets to extend this.
	if enum != 0 {
		panic(fmt.Sprintf("Invalid TFFeatures enum, residual bits :%d", enum))
	}

	return strings.Join(ret, "And")
}

// FixtureID is a convenience function to be used in the test registration.
func FixtureID(enum TFFeatures) string {
	return enum.String()
}

// tastFixtureImpl is the Tast implementation of the Wificell fixture.
// Notice the difference between tastFixtureImpl and TestFixture objects.
// The former is the one in the Tast framework; the latter is for
// wificell fixture.
type tastFixtureImpl struct {
	features TFFeatures
	tf       *TestFixture
	rtd      []*linuxssh.RemoteFileDelta
}

// newTastFixture creates a Tast fixture with given features.
func newTastFixture(features TFFeatures) *tastFixtureImpl {
	return &tastFixtureImpl{
		features: features,
	}
}

// companionName returns the hostname of a companion device.
func (f *tastFixtureImpl) companionName(s *testing.FixtState, suffix string) string {
	name, err := utils.CompanionDeviceHostname(s.DUT().HostName(), suffix)
	if err != nil {
		s.Fatal("Unable to synthesize name, err: ", err)
	}
	return name
}

// dutHealthCheck checks if the DUT is healthy.
func (f *tastFixtureImpl) dutHealthCheck(ctx context.Context, d *dut.DUT, rpcHint *testing.RPCHint) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// We create a new gRPC session here to exclude broken gRPC case and save reboots when
	// the DUT is healthy but the gRPC is broken.
	rpcClient, err := rpc.Dial(ctx, d, rpcHint)
	if err != nil {
		return errors.Wrap(err, "cannot create gRPC client")
	}
	defer rpcClient.Close(ctx)

	wifiClient := wifi.NewShillServiceClient(rpcClient.Conn)
	if _, err := wifiClient.HealthCheck(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "health check failed")
	}
	return nil
}

// recoverUnhealthyDUT checks if the DUT is healthy. If not, try to recover it
// with reboot.
func (f *tastFixtureImpl) recoverUnhealthyDUT(ctx context.Context, d *dut.DUT, s *testing.FixtState) error {
	if !d.Connected(ctx) {
		testing.ContextLog(ctx, "DUT found to not be connected before health check; reconnecting to DUT")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := d.WaitConnect(ctx); err != nil {
				return errors.Wrap(err, "failed to connect to DUT")
			}
			return nil
		}, &testing.PollOptions{
			Timeout: 1 * time.Minute,
		}); err != nil {
			return errors.Wrap(err, "failed to wait for DUT to connect")
		}
		testing.ContextLog(ctx, "Successfully reconnected to DUT")
	}
	if err := f.dutHealthCheck(ctx, d, s.RPCHint()); err != nil {
		testing.ContextLog(ctx, "Rebooting the DUT due to health check err: ", err)
		// As reboot will at least break tf.rpc, no reason to keep
		// the existing p.tf. Close it before reboot.
		if f.tf != nil {
			testing.ContextLog(ctx, "Close TestFixture before reboot")
			if err := f.tf.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close TestFixture before DUT reboot recovery: ", err)
			}
			f.tf = nil
		}
		if err := d.Reboot(ctx); err != nil {
			return errors.Wrap(err, "reboot failed")
		}
	}
	return nil
}

// takeIdleWiFiMeasurement deactivates all WiFi interfaces on the DUT and takes
// power measurements to be used as a baseline of comparison for WiFi scenarios.
func (f *tastFixtureImpl) takeIdleWiFiMeasurement(ctx context.Context, s *testing.FixtState) error {
	if f.tf.powerRecorderClient == nil {
		return errors.New("no power recorder client available")
	}
	ctx, restore, err := f.tf.RemoveWiFiInterfaces(ctx, DefaultDUT)
	if _, err := f.tf.powerRecorderClient.Start(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to start metrics")
	}

	defer func() {
		if err := restore(); err != nil {
			s.Error("Failed to restore WiFi interfaces: ", err)
		}
	}()

	// GoBigSleepLint: We want to take a power measurement over |idlePowerSleepTime| of idle time.
	if err := testing.Sleep(ctx, idlePowerSleepTime); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	result, err := f.tf.powerRecorderClient.Stop(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to stop metrics")
	}
	f.tf.idlePowerValues = perf.NewValuesFromProto(result.GetPerfMetrics())

	return nil
}

// setUpPower sets up the RPC link and configuration to allow power measurements
// during the test. It also initiates an idle power measurement to be used as a baseline.
func (f *tastFixtureImpl) setUpPower(ctx context.Context, s *testing.FixtState) error {
	cl := f.tf.duts[DefaultDUT].rpc

	// Set up device for power test.
	f.tf.powerSetupClient = power.NewDeviceSetupServiceClient(cl.Conn)
	setupRequest := power.DeviceSetupRequest{
		Ui:                 power.UIMode_DISABLE_UI,
		ScreenBrightness:   power.ScreenMode_ZERO_SCREEN_BRIGHTNESS,
		KeyboardBrightness: power.KeyboardMode_ZERO_KEYBOARD_BRIGHTNESS,
	}
	var err error
	if _, err = f.tf.powerSetupClient.Setup(ctx, &setupRequest); err != nil {
		return errors.Wrap(err, "failed to setup device for power test")
	}
	deviceCleanup := func(ctx context.Context) error {
		if _, err = f.tf.powerSetupClient.Cleanup(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to restore device setup")
		}
		return nil
	}
	defer func() {
		if err != nil || s.HasError() {
			deviceCleanup(ctx)
		}
	}()

	// Create recorder for power measurement.
	f.tf.powerRecorderClient = power.NewRecorderServiceClient(cl.Conn)
	recorderRequest := power.RecorderRequest{
		IntervalSec: 5,
	}
	if _, err = f.tf.powerRecorderClient.Create(ctx, &recorderRequest); err != nil {
		return errors.Wrap(err, "failed to create recorder for power test")
	}
	recorderCleanup := func(ctx context.Context) error {
		if _, err = f.tf.powerRecorderClient.Close(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to close recorder")
		}
		return nil
	}
	defer func() {
		if err != nil || s.HasError() {
			recorderCleanup(ctx)
		}
	}()

	if err := f.takeIdleWiFiMeasurement(ctx, s); err != nil {
		return errors.Wrap(err, "failed to take idle WiFi measurement")
	}

	f.tf.powerCleanup = func(ctx context.Context) error {
		if err := recorderCleanup(ctx); err != nil {
			return err
		}
		if err := deviceCleanup(ctx); err != nil {
			return err
		}
		return nil
	}
	return nil
}

// savePowerResults calculates the average difference between |results| and the stored
// idle values, and saves them to the output directory so they can be pushed to
// Crosbolt.
func (f *tastFixtureImpl) savePowerResults(ctx context.Context, s *testing.FixtTestState, results *perf.Values) error {
	if f.tf.idlePowerValues == nil {
		return errors.New("no idle power results avaliable")
	}
	metricDeltas := perf.NewValues()
	for metric, values := range results.GetValues() {
		average := wifiutil.Average(values)

		idleValues := f.tf.idlePowerValues.GetValueByMetric(metric)
		if idleValues == nil {
			continue
		}
		idleAverage := wifiutil.Average(idleValues)

		delta := average - idleAverage
		metricDeltas.Set(metric, delta)
	}
	if err := metricDeltas.Save(s.OutDir()); err != nil {
		return errors.Wrap(err, "failed to save perf values")
	}
	return nil
}

func (f *tastFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if f.features&TFFeaturesEnroll != 0 {
		// Do this before NewTestFixture as DUT might be rebooted which will break tf.rpc.
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to reset TPM: ", err)
		}
	}

	if err := f.recoverUnhealthyDUT(ctx, s.DUT(), s); err != nil {
		s.Fatal("Failed to recover unhealthy DUT: ", err)
	}

	// Create TestFixture.
	ops := NewTFOptionsBuilder()
	ops.DutTarget(s.DUT(), s.RPCHint())

	// Read fixture vars for router host(s) identification.
	if f.features&TFFeaturesRouters != 0 {
		var routers []string
		if routersStr, ok := s.Var(fixtureVarRoutersMultiple); ok && routersStr != "" {
			testing.ContextLog(ctx, "routers: ", routersStr)
			routers = strings.Split(routersStr, ",")
			if len(routers) < 2 {
				s.Fatal("Must provide at least two router names when Routers feature is enabled")
			}
		} else {
			router, routerOk := s.Var(fixtureVarRouter)
			pcap, pcapOk := s.Var(fixtureVarPcap)
			if routerOk && pcapOk && router != "" && pcap != "" {
				routers = []string{router, pcap}
			} else {
				for _, suffix := range []string{utils.CompanionSuffixRouter, utils.CompanionSuffixPcap} {
					routers = append(routers, f.companionName(s, suffix))
				}
			}
		}
		ops.PrimaryRouterTargets(routers...)
	} else {
		router, ok := s.Var(fixtureVarRouter)
		if ok && router != "" {
			ops.PrimaryRouterTargets(router)
		} // else: let TestFixture resolve the name.
	}

	// Read fixture vars and configures settings related to packet capturing.
	if f.features&TFFeaturesCapture != 0 {
		ops.EnablePacketCapture(true)
	}
	if f.features&TFFeaturesRouterAsCapture != 0 {
		ops.UseFirstRouterAsPcap(true)
	} else {
		pcap, ok := s.Var(fixtureVarPcap)
		if ok && pcap != "" {
			ops.PcapRouterTarget(pcap)
		} // else: let TestFixture resolve the name.
	}

	// Read fixture var for attenuator host.
	if f.features&TFFeaturesAttenuator != 0 {
		atten, ok := s.Var(fixtureVarAttenuator)
		if !ok || atten == "" {
			// Attenuator is not typical companion, so we synthesize its name here.
			atten = f.companionName(s, "-attenuator")
		}
		ops.AttenuatorTarget(atten)
	}

	// Read companion DUT.
	if f.features&TFFeaturesCompanionDUT != 0 {
		ops.RequirePrimaryRouter(false)

		for role, cd := range s.CompanionDUTs() {
			if cd == nil {
				s.Fatalf("Failed to get companion DUT %s", role)
			}
			ops.DutTarget(cd, s.RPCHint())
			if err := f.recoverUnhealthyDUT(ctx, cd, s); err != nil {
				s.Fatalf("Failed to recover unhealthy DUT %s: %s", role, err)
			}
		}
	}

	if f.features&TFFeaturesCellular != 0 {
		ops.EnableCellular(true)
	}
	if f.features&TFFeaturesWithUI != 0 {
		ops.EnableDutUI(true)
	}

	if enableRouterRebootStr, ok := s.Var(fixtureVarEnableRouterReboot); ok {
		enableRouterReboot, err := strconv.ParseBool(enableRouterRebootStr)
		if err != nil {
			s.Fatalf("Failed to parse bool fixture var %q: %v", fixtureVarEnableRouterReboot, err)
		}
		ops.EnableRouterReboot(enableRouterReboot)
	}

	tf, err := NewTestFixture(ctx, s.FixtContext(), ops.Build())
	if err != nil {
		s.Fatal("Failed to set up test fixture: ", err)
	}
	f.tf = tf

	if f.features&TFFeaturesEnroll != 0 {
		for i := range f.tf.duts {
			// TODO(b/243629567): Remove the retries when the enroll fixture is stable enough.
			ok := false
			for tries := 1; tries <= enrollRetry; tries++ {
				// Make sure we have enough time to perform enrollment.
				// This helps differentiate real issues from timeout hitting different components.
				if deadline, ok := ctx.Deadline(); !ok {
					s.Fatal("Missing deadline for context: ", ctx)
				} else if diff := deadline.Sub(time.Now()); diff < enrollmentRunTimeout {
					s.Fatalf("Not enought time to perform setup and enrollment: have %s; need %s", diff, enrollmentRunTimeout)
				}

				s.Logf("Attempting enrollment, try %d/%d", tries, enrollRetry)
				attemptDir := path.Join(s.OutDir(), fmt.Sprintf("Attempt_%d", tries))
				enrollCtx, cancel := context.WithTimeout(ctx, enrollmentRunTimeout)
				defer cancel()

				// The resulting FakeDMS directory is not yet used. If needed it can be passed along in tf for each DUT.
				_, err := policyutil.Enroll(enrollCtx, attemptDir, f.tf.duts[i].dut, f.tf.duts[i].rpc, false)
				if err != nil {
					s.Logf("Attempt %d failed", tries)
				} else {
					// When the enrollment is successful, there is no need to retry again.
					s.Logf("Attempt %d succeded", tries)
					ok = true
					break
				}
			}
			if !ok {
				s.Fatal("Failed to enroll Chrome")
			}
		}
	}

	if f.features&TFFeaturesPower != 0 {
		if err := f.setUpPower(ctx, s); err != nil {
			s.Fatal("Failed to set up power measurement: ", err)
		}
	}

	return f.tf
}

func (f *tastFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.features&TFFeaturesPower != 0 {
		if f.tf.powerCleanup == nil {
			s.Error("No power cleanup function available")
		}
		if err := f.tf.powerCleanup(ctx); err != nil {
			s.Error("Power cleanup failed: ", err)
		}
	}

	duts := f.tf.duts // Make a copy of the slice to iterate over.
	for _, d := range duts {
		if f.features&TFFeaturesEnroll != 0 {
			pc := policy.NewPolicyServiceClient(d.rpc.Conn)

			if _, err := pc.StopChromeAndFakeDMS(ctx, &empty.Empty{}); err != nil {
				s.Error("Failed to close Chrome instance and Fake DMS: ", err)
			}

			// Reset DUT TPM and system state to leave it in a good state post test.
			if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, d.dut, s.RPCHint()); err != nil {
				s.Error("Failed to reset TPM: ", err)
			}
		}
		// Ensure DUT is healthy here again, so that we don't leave with
		// bad state to later tests/tasks.
		if err := f.recoverUnhealthyDUT(ctx, d.dut, s); err != nil {
			s.Fatal("Failed to recover unhealthy DUT: ", err)
		}
	}

	if f.tf == nil {
		return
	}

	if err := f.tf.Close(ctx); err != nil {
		s.Log("Failed to tear down test fixture, err: ", err)
	}
	f.tf = nil
}

func (f *tastFixtureImpl) Reset(ctx context.Context) error {
	if err := f.tf.Reinit(ctx); err != nil {
		return errors.Wrap(err, "failed to reinit test fixture")
	}
	return nil
}

func (f *tastFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	const maxLogSize = 20 * 1024 * 1024 //20mb

	//Create rtd for Main DUT
	if file, err := linuxssh.NewRemoteFileDelta(ctx, s.DUT().Conn(), "/var/log/net.log", filepath.Join(s.OutDir(), fmt.Sprintf("net_%s.log", s.TestName())), maxLogSize); err != nil {
		s.Fatal("File transfer failed: ", err)
	} else {
		f.rtd = append(f.rtd, file)
	}

	//Create rtd for CompanionDUTs
	for role, cd := range s.CompanionDUTs() {
		if file, err := linuxssh.NewRemoteFileDelta(ctx, cd.Conn(), "/var/log/net.log", filepath.Join(s.OutDir(), fmt.Sprintf("net_%s_%s.log", s.TestName(), role)), maxLogSize); err != nil {
			s.Fatal("File transfer failed: ", err)
		} else {
			f.rtd = append(f.rtd, file)
		}
	}

	if f.features&TFFeaturesSelfManagedAP != 0 {
		if err := f.tf.SeedRegdomain(ctx); err != nil {
			s.Fatal("Failed to configure Regdomain seeding AP: ", err)
		}
	}

	if f.features&TFFeaturesPower != 0 {
		if _, err := f.tf.powerRecorderClient.Start(s.TestContext(), &empty.Empty{}); err != nil {
			s.Fatal("Failed to start power metrics: ", err)
		}
	}
}

func (f *tastFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	//Save all rtds
	if f.rtd != nil {
		for _, rtd := range f.rtd {
			defer rtd.Save(ctx)
		}
	}

	if f.features&TFFeaturesPower != 0 {
		response, err := f.tf.powerRecorderClient.Stop(s.TestContext(), &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to stop power metrics: ", err)
		}
		powerResults := perf.NewValuesFromProto(response.GetPerfMetrics())
		if err = f.savePowerResults(ctx, s, powerResults); err != nil {
			s.Fatal("Failed to save power results: ", err)
		}
	}

	if f.features&TFFeaturesSelfManagedAP != 0 {
		if err := f.tf.DeconfigSeedingAP(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to deconfig seeding AP: ", err) // Do nothing else, the primary error is more important.
		}
	}

	if err := f.tf.CollectLogs(ctx); err != nil {
		s.Log("Error collecting logs, err: ", err)
	}
}
