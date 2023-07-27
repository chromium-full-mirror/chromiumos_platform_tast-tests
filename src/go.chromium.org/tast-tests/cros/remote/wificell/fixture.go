// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/network/iw"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/wificell/router/common/support"
	"go.chromium.org/tast-tests/cros/remote/wificell/wifiutil"
	"go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
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
	postTestTimeout      = 5 * time.Second
	enrollmentRunTimeout = 4 * time.Minute
	enrollRetry          = 3
	idlePowerSleepTime   = 50 * time.Second
	powerSetUpTimeout    = 2*idlePowerSleepTime + setUpTimeout
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
	params[TFFeaturesRouters|TFFeaturesAttenuator] = "WiFi romaing setup with multiple routers and attenuators"
	params[TFFeaturesEnroll] = "Wificell setup with router and pcap object and chrome enrolled"
	params[TFFeaturesCompanionDUT] = "Wificell setup with companion Chromebook DUT"
	params[TFFeaturesCompanionDUT|TFFeaturesCapture] = "Wificell setup with companion Chromebook DUT and packet capture from the pcap device"
	params[TFFeaturesPower] = "Default wificell setup with power diagnostics"
	params[TFFeaturesCellular] = "Wificell setup on a cellular capable device"
	params[TFFeaturesCompanionDUT|TFFeaturesCellular] = "Wificell setup on a cellular capable device with companion chromebook DUT"

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
			PostTestTimeout: postTestTimeout,
			TearDownTimeout: tearDownTimeout,
			ServiceDeps:     []string{ShillServiceName, BluetoothServiceName},
			Vars:            []string{"router", "routertype"},
		}

		// Typical fixture extensions.
		if f&TFFeaturesCapture != 0 {
			fixtures[f].Vars = append(fixtures[f].Vars, "pcap", "pcaptype")
		}
		if f&TFFeaturesAttenuator != 0 {
			fixtures[f].Vars = append(fixtures[f].Vars, "attenuator")
		}
		if f&TFFeaturesPower != 0 {
			fixtures[f].ServiceDeps = append(fixtures[f].ServiceDeps, PowerServiceName)
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
	// TFFeaturesPower is a feature that enables power measurements.
	TFFeaturesPower
	// TFFeaturesCellular set up cellular shill service on the DUT.
	TFFeaturesCellular
)

// String returns name component corresponding to enum value(s).
func (enum TFFeatures) String() string {
	if enum == 0 {
		return "wificellFixt"
	}
	ret := []string{"wificellFixt"}
	if enum&TFFeaturesCapture != 0 {
		ret = append(ret, "WithCapture")
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
	if enum&TFFeaturesPower != 0 {
		ret = append(ret, "WithPower")
		enum ^= TFFeaturesPower
	}
	if enum&TFFeaturesCellular != 0 {
		ret = append(ret, "WithCellular")
		enum ^= TFFeaturesCellular
	}
	// Catch weird cases. Like when somebody extends enum, but forgets to extend this.
	if enum != 0 {
		panic(fmt.Sprintf("Invalid TFFeatures enum, residual bits :%d", enum))
	}

	return strings.Join(ret, "")
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
	if f.tf.powerClient == nil {
		return errors.New("no power client available")
	}
	ctx, restore, err := f.tf.RemoveWiFiInterfaces(ctx, DefaultDUT)
	if _, err := f.tf.powerClient.Start(ctx, &empty.Empty{}); err != nil {
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

	request := power.FinishRequest{Upload: false}
	values, err := f.tf.powerClient.Finish(ctx, &request)
	if err != nil {
		return errors.Wrap(err, "failed to stop metrics")
	}
	f.tf.idlePowerValues = perf.NewValuesFromProto(values)

	return nil
}

// setUpPower sets up the RPC link and configuration to allow power measurements
// during the test. It also initiates an idle power measurement to be used as a baseline.
func (f *tastFixtureImpl) setUpPower(ctx context.Context, s *testing.FixtState) error {
	cl := f.tf.duts[DefaultDUT].rpc
	f.tf.powerClient = power.NewMetricsServiceClient(cl.Conn)
	setupRequest := power.SetupRequest{Fixture: power.SetupRequest_NO_UI_WIFI, IntervalSecond: 5}
	var err error = nil
	if _, err = f.tf.powerClient.Setup(ctx, &setupRequest); err != nil {
		return errors.Wrap(err, "failed to setup metrics")
	}
	cleanup := func(ctx context.Context) error {
		if _, err = f.tf.powerClient.Cleanup(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to cleanup metrics")
		}
		return nil
	}
	defer func() {
		if err != nil || s.HasError() {
			cleanup(ctx)
		}
	}()
	if err := f.takeIdleWiFiMeasurement(ctx, s); err != nil {
		return errors.Wrap(err, "failed to take idle WiFi measurement")
	}

	f.tf.powerCleanup = cleanup
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
	var ops []TFOption
	// Read router/pcap variable. If not available or empty, NewTestFixture
	// will fall back to Default{Router,Pcap}Host.
	if f.features&TFFeaturesRouters != 0 {
		if routers, ok := s.Var("routers"); ok && routers != "" {
			testing.ContextLog(ctx, "routers: ", routers)
			slice := strings.Split(routers, ",")
			if len(slice) < 2 {
				s.Fatal("Must provide at least two router names when Routers feature is enabled")
			}
			ops = append(ops, TFRouter(slice...))
		} else {
			var routers []string
			for _, suffix := range []string{utils.CompanionSuffixRouter, utils.CompanionSuffixPcap} {
				routers = append(routers, f.companionName(s, suffix))

			}
			testing.ContextLog(ctx, "companion routers: ", routers)
			ops = append(ops, TFRouter(routers...))
		}
	} else {
		router, ok := s.Var("router")
		if ok && router != "" {
			testing.ContextLog(ctx, "router: ", router)
			ops = append(ops, TFRouter(router))
		} // else: let TestFixture resolve the name.
	}
	pcap, ok := s.Var("pcap")
	if ok && pcap != "" {
		testing.ContextLog(ctx, "pcap: ", pcap)
		ops = append(ops, TFPcap(pcap))
	} // else: let TestFixture resolve the name.
	if f.features&TFFeaturesRouterAsCapture != 0 {
		testing.ContextLog(ctx, "using router as pcap")
		ops = append(ops, TFRouterAsCapture())
	}
	// Read attenuator variable.
	if f.features&TFFeaturesAttenuator != 0 {
		atten, ok := s.Var("attenuator")
		if !ok || atten == "" {
			// Attenuator is not typical companion, so we synthesize its name here.
			atten = f.companionName(s, "-attenuator")
		}
		testing.ContextLog(ctx, "attenuator: ", atten)
		ops = append(ops, TFAttenuator(atten))
	}
	// Enable capturing.
	if f.features&TFFeaturesCapture != 0 {
		ops = append(ops, TFCapture(true))
	}

	// Allow for setting router type
	var routerType support.RouterType
	if rTypeStr, ok := s.Var("routertype"); !ok || rTypeStr == "" {
		// Default to unknown so that it may be automatically determined with host
		routerType = support.UnknownT
	} else {
		var err error
		routerType, err = support.ParseRouterType(rTypeStr)
		if err != nil {
			s.Fatalf("Failed to parse routertype %q: ", err)
		}
	}
	testing.ContextLog(ctx, "routertype: ", routerType.String())
	ops = append(ops, TFRouterType(routerType))

	// Allow for setting pcap type
	var pcapType support.RouterType
	if rTypeStr, ok := s.Var("pcaptype"); !ok || rTypeStr == "" {
		// Default to unknown so that it may be automatically determined with host
		pcapType = support.UnknownT
	} else {
		var err error
		pcapType, err = support.ParseRouterType(rTypeStr)
		if err != nil {
			s.Fatalf("Failed to parse pcaptype %q: ", err)
		}
	}
	testing.ContextLog(ctx, "pcaptype: ", pcapType.String())
	ops = append(ops, TFPcapType(pcapType))

	// Read companion DUT.
	if f.features&TFFeaturesCompanionDUT != 0 {
		cd := s.CompanionDUT("cd1")
		if cd == nil {
			s.Fatal("Failed to get companion DUT cd1")
		}
		ops = append(ops, TFRouterRequired(false))
		ops = append(ops, TFCompanionDUT(cd, s.RPCHint()))
		if err := f.recoverUnhealthyDUT(ctx, cd, s); err != nil {
			s.Fatal("Failed to recover unhealthy DUT: ", err)
		}
	}

	if f.features&TFFeaturesCellular != 0 {
		ops = append(ops, TFCellular())
	}

	tf, err := NewTestFixture(ctx, s.FixtContext(), s.DUT(), s.RPCHint(), ops...)
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

	if f.features&TFFeaturesCompanionDUT != 0 {
		// Set DUT regulatory domain to US for all testcases.
		iwr := iw.NewRemoteRunner(tf.DUTConn(DefaultDUT))
		selfManaged, err := iwr.IsRegulatorySelfManaged(ctx)
		if err != nil {
			s.Fatal("Failed to read regulatory status: ", err)
		}

		if selfManaged {
			// For self-managed solution, seed the region through AP 802.11d IE.
			if err := tf.SeedRegdomain(ctx, DefaultDUT); err != nil {
				s.Fatal("Failed to configure Regdomain seeding AP: ", err)
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
	if f.features&TFFeaturesCompanionDUT != 0 {
		if err := f.tf.DeconfigSeedingAP(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to deconfig seeding AP: ", err) // Do nothing else, the primary error is more important.
		}
	}
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
	if f.features&TFFeaturesPower == 0 {
		return
	}
	if _, err := f.tf.powerClient.Start(s.TestContext(), &empty.Empty{}); err != nil {
		s.Fatal("Failed to start power metrics: ", err)
	}

}

func (f *tastFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.features&TFFeaturesPower != 0 {
		request := power.FinishRequest{Upload: false}
		values, err := f.tf.powerClient.Finish(s.TestContext(), &request)
		if err != nil {
			s.Fatal("Failed to stop power metrics: ", err)
		}
		powerResults := perf.NewValuesFromProto(values)
		if err = f.savePowerResults(ctx, s, powerResults); err != nil {
			s.Fatal("Failed to save power results: ", err)
		}
	}

	if err := f.tf.CollectLogs(ctx); err != nil {
		s.Log("Error collecting logs, err: ", err)
	}
}
