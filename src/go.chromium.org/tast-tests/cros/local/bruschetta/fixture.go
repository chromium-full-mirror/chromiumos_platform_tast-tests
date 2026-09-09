// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bruschetta/constants"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/devicemode"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/vm"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	installationTimeout   = 40 * time.Minute
	resetTimeout          = time.Minute
	preTestTimeout        = 15 * time.Second
	postTestTimeout       = 30 * time.Second
	uninstallationTimeout = 2 * time.Minute

	// referenceVMInstaller is the installer image for the reference VM.
	referenceVMInstaller     = "refvm.qcow2"
	referenceVMInstallerHash = "refvm.qcow2.SHA256"

	// referenceVMPflash is the pflash image for the reference VM.
	referenceVMPflash     = "refvm_VARS.fd"
	referenceVMPflashHash = "refvm_VARS.fd.SHA256"

	// BruschettaFixture is the name of the fixture with ash.
	BruschettaFixture = "bruschettaReferenceVM"
	// BruschettaFixtureWithVMConfig is the name of the fixture that assumes a custom VM config other than refvm.
	BruschettaFixtureWithVMConfig = "bruschettaFixtureWithCustomVMConfig"
	// BruschettaFixtureClamshell is the name of the fixture with ash, only in clamshell mode.
	BruschettaFixtureClamshell = "bruschettaReferenceVMClamshell"
	// BruschettaFixtureWithFieldtrialConfig is the name of the fixture with ash
	// and fieldtrial config enabled.
	BruschettaFixtureWithFieldtrialConfig = "bruschettaReferenceVMWithFieldtrialConfig"

	defaultVMName = "bru"

	httpAddr          = "localhost:12345"
	httpInstallerPath = "files/refvm.qcow2"
	httpPflashPath    = "files/refvm_VARS.fd"
)

var bruschettaVMConfigurationVar = testing.RegisterVarString(
	"bruschetta.vm_configuration",
	"",
	"Path to a bruschetta VM configuration on the DUT. This is a json object representing a BruschettaVMConfiguration entry, see below refvmConfiguratoin func for a good example. If unset the default refvm configuration is used.",
)

var bruschettaKeepVM = testing.RegisterVarString(
	"bruschetta.keep_vm",
	"false",
	"Boolean to indicate that the Bruschetta instance should be kept when the fixture is cleaned up, otherwise it is removed.",
)

// BruschettaHwDeps prevents tests from running on devices without enough storage or RAM.
var BruschettaHwDeps = hwdep.D(
	hwdep.MinStorage(50),
	hwdep.MinMemory(7*1024),
	// Some Bruschetta tests are failing on reven (flex) boards (b/407460862),
	// also as Bruschetta won't be enabled on reven in production so it should
	// be fine to just skip this board.
	hwdep.SkipOnModel("reven"),
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            BruschettaFixture,
		Desc:            "Set up reference VM",
		Contacts:        []string{"clumptini+oncall@google.com"},
		BugComponent:    "b:658562", // ChromeOS > Software > GuestOS
		Impl:            &bruschettaFixture{},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Data:            []string{referenceVMInstaller, referenceVMInstallerHash, referenceVMPflash, referenceVMPflashHash},
		Parent:          fixture.ChromePolicyLoggedInBruschetta,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            BruschettaFixtureWithVMConfig,
		Desc:            "Set up reference VM assuming a non-refvm VM config is provided",
		Contacts:        []string{"clumptini+oncall@google.com"},
		BugComponent:    "b:658562", // ChromeOS > Software > GuestOS
		Impl:            &bruschettaFixture{},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          fixture.ChromePolicyLoggedInBruschetta,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            BruschettaFixtureClamshell,
		Desc:            "Set up reference VM in clamshell mode",
		Contacts:        []string{"clumptini+oncall@google.com"},
		BugComponent:    "b:658562", // ChromeOS > Software > GuestOS
		Impl:            &bruschettaAppsFixture{deviceMode: devicemode.ClamshellMode},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    resetTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Data:            []string{referenceVMInstaller, referenceVMInstallerHash, referenceVMPflash, referenceVMPflashHash},
		Parent:          BruschettaFixture,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            BruschettaFixtureWithFieldtrialConfig,
		Desc:            "Set up reference VM with fieldtrial config enabled",
		Contacts:        []string{"clumptini+oncall@google.com"},
		BugComponent:    "b:658562", // ChromeOS > Software > GuestOS
		Impl:            &bruschettaFixture{},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    resetTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Data:            []string{referenceVMInstaller, referenceVMInstallerHash, referenceVMPflash, referenceVMPflashHash},
		Parent:          fixture.ChromePolicyLoggedInBruschettaWithFieldtrialConfig,
	})
}

// bruschettaFixture holds the runtime state of the fixture.
type bruschettaFixture struct {
	// fakeDMS is an already running DMS server.
	fakeDMS *fakedms.FakeDMS
	// chrome is a logged in chrome instance that loads policies from fakeDMS.
	chrome *chrome.Chrome
	// policy is the base-line policy all tests start with.
	policy *policy.BruschettaVMConfiguration
	// vm is the running VM instance.
	vm *vm.VM
	// tconn is the test connection to chrome.
	tconn *chrome.TestConn
	// How far into the VM log we've read.
	logOffset        int
	bruschettaVM     *vm.BruschettaVM
	kb               *input.KeyboardEventWriter
	concierge        *vm.Concierge
	releaseKeepAwake func(ctx context.Context, tconn *chrome.TestConn) error
}

type bruschettaAppsFixture struct {
	tconn            *chrome.TestConn
	deviceMode       devicemode.DeviceMode
	revertDeviceMode func(ctx context.Context) error
}

// FixtureData is the data returned by SetUp and passed to tests.
type FixtureData struct {
	// FakeDMS is an already running DMS server. At the start of each test it will
	// serve a baseline bruschetta policy which allows the VM to run. Modifying this
	// policy during a test is okay.
	FakeDMS *fakedms.FakeDMS
	// Chrome is a logged in chrome instance that loads policies from fakeDMS.
	Chrome *chrome.Chrome
	// VM is the running VM instance.
	VM *vm.VM
	// Tconn is the test connection to Chrome.
	Tconn        *chrome.TestConn
	BruschettaVM *vm.BruschettaVM
	KB           *input.KeyboardEventWriter
}

func refvmConfiguration(s *testing.FixtState) map[string]interface{} {
	imageHash, err := os.ReadFile(s.DataPath(referenceVMInstallerHash))
	if err != nil {
		s.Fatal("Failed to read disk image hash: ", err)
	}

	pflashHash, err := os.ReadFile(s.DataPath(referenceVMPflashHash))
	if err != nil {
		s.Fatal("Failed to read pflash hash: ", err)
	}

	return map[string]interface{}{
		"glinux-latest": map[string]interface{}{
			"name":          constants.BruschettaVMName,
			"enabled_state": "INSTALL_ALLOWED",
			"installer_image_x86_64": map[string]interface{}{
				"url":  fmt.Sprintf("http://%s/%s", httpAddr, httpInstallerPath),
				"hash": strings.TrimSpace(string(imageHash)),
			},
			"uefi_pflash_x86_64": map[string]interface{}{
				"url":  fmt.Sprintf("http://%s/%s", httpAddr, httpPflashPath),
				"hash": strings.TrimSpace(string(pflashHash)),
			},
			"vtpm": map[string]interface{}{
				"enabled":              true,
				"policy_update_action": "NONE",
			},
			"oem_strings": []interface{}{
				"refvm:install=true",
				"refvm:noninteractive=true",
			},
		},
	}
}

func (f *bruschettaFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Use a shortened context for setup operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, uninstallationTimeout)
	defer cancel()

	f.fakeDMS = s.ParentValue().(*fixtures.FixtData).FakeDMS()
	f.chrome = s.ParentValue().(*fixtures.FixtData).Chrome()
	tconn, err := f.chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	f.tconn = tconn

	// If a custom VM config is specified then we assume serving those artifacts is handled externally.
	if bruschettaVMConfigurationVar.Value() == "" {
		s.Log("Starting local HTTP server")

		// Use cleanupCtx here because the HTTP server needs to use the cleanup time for shutdown.
		shutdownServer := startHTTPServer(cleanupCtx, s.DataPath(referenceVMInstaller), s.DataPath(referenceVMPflash))
		defer func() {
			s.Log("Stopping local HTTP server")
			shutdownServer()
		}()
	}

	s.Log("Setting chrome policy")

	var configVal map[string]interface{}
	if cfgPath := bruschettaVMConfigurationVar.Value(); cfgPath != "" {
		s.Logf("Loading bruschetta vm configuration from %q", cfgPath)

		b, err := os.ReadFile(cfgPath)
		if err != nil {
			s.Fatalf("Failed to read bruschetta.vm_configuration file %q: %v", cfgPath, err)
		}
		if err := json.Unmarshal(b, &configVal); err != nil {
			s.Fatalf("Failed to unmarshal %q as json: %v", cfgPath, err)
		}
	} else {
		s.Log("Using default refvm configuration")
		configVal = refvmConfiguration(s)
	}

	f.policy = &policy.BruschettaVMConfiguration{
		Stat: policy.StatusSet,
		Val:  configVal,
	}

	if err := policyutil.ServeAndVerify(ctx, f.fakeDMS, f.chrome, []policy.Policy{f.policy}); err != nil {
		s.Fatal("Failed to serve bruschetta policy to chrome: ", err)
	}

	if err := guestos.SetSolidColorWallpaper(ctx, f.tconn); err != nil {
		s.Log("Failed to change wallpaper: ", err)
	}

	concierge, err := vm.GetRunningConcierge(ctx, f.chrome.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get concierge: ", err)
	}
	f.concierge = concierge

	newVM, err := vm.NewSystemRecognizedVM(f.concierge, false, 0, vm.Bruschetta)
	if err != nil {
		s.Fatal("Failed to get VM object: ", err)
	}
	f.vm = newVM
	f.bruschettaVM = &vm.BruschettaVM{VM: newVM}

	// Skip past logs that might be left over from previous tests.
	existingLogs, err := f.vm.RetrieveLogs(ctx)
	if err != nil {
		s.Fatal("Failed to get existing VM logs: ", err)
	}
	f.logOffset = len(existingLogs)

	defer func(ctx context.Context) {
		if err := f.saveLogs(ctx, s.OutDir(), "setup"); err != nil {
			s.Fatal("Failed to save VM logs from fixture setup: ", err)
		}
	}(cleanupCtx)

	s.Log("Installing VM")

	if err := installBruschetta(ctx, f.tconn); err != nil {
		s.Fatal("Failed to install VM: ", err)
	}
	defer func(ctx context.Context) {
		keepVM, err := strconv.ParseBool(bruschettaKeepVM.Value())
		if err != nil {
			s.Errorf("Failed to parse bruschetta.keep_vm value %q, assuming false: %v", bruschettaKeepVM.Value(), err)
			keepVM = false
		}
		if !s.HasError() || keepVM {
			return
		}

		if err := removeBruschetta(ctx, f.tconn); err != nil {
			s.Fatal("Failed to remove VM after setup failure: ", err)
		}
	}(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, s.OutDir(), s.HasError, tconn, "ui_tree")

	vms, err := f.concierge.ListVms(ctx)
	if err != nil {
		s.Fatal("Failed to list VMs: ", err)
	}

	// b/343813416: Forward-compatible for chromium change crrev/c/7700640.
	// In the old behavior, the installation step returns as soon as the installation window "opens".
	// The fixture needs to wait for the window to close when the installer VM is done.
	// In the new behavior, the installation step returns after the installation is closed.
	// As a result, we don't wait for VM to stop.
	installerRunning := false
	for _, v := range vms {
		if v.GetName() == f.vm.Name() {
			installerRunning = true
			break
		}
	}

	if installerRunning {
		s.Log("Installer VM is running, waiting for it to stop")
		if err := f.concierge.WaitForVMStop(ctx, f.vm); err != nil {
			s.Fatal("Failed to wait for VM to finish installing: ", err)
		}
		// Now use the terminal app to boot the VM.
		s.Log("Starting installed VM")
		if _, err := terminalapp.FindBruschetta(ctx, f.tconn); err != nil {
			s.Fatal("Failed to start bruschetta VM using terminal app: ", err)
		}
	} else {
		s.Log("Installer VM is already closed, waiting for VM to start")
		if _, err := terminalapp.WaitForBruschettaPrompt(ctx, f.tconn); err != nil {
			s.Fatal("Failed to wait for bruschetta VM to start: ", err)
		}
	}

	if err = apps.Close(ctx, f.tconn, apps.Terminal.ID); err != nil {
		s.Fatal("Failed to close terminal app: ", err)
	}

	if err := f.concierge.GetVMInfo(ctx, f.vm); err != nil {
		s.Fatal("Failed to get running VM info: ", err)
	}

	if f.kb, err = input.Keyboard(ctx); err != nil {
		s.Fatal("Failed to create keyboard device: ", err)
	}

	if err := guestos.DisableCursorBlinking(ctx, f.bruschettaVM); err != nil {
		s.Fatal("Failed to disable cursor blinking: ", err)
	}

	return FixtureData{
		FakeDMS:      f.fakeDMS,
		Chrome:       f.chrome,
		Tconn:        f.tconn,
		VM:           f.vm,
		BruschettaVM: f.bruschettaVM,
		KB:           f.kb,
	}
}

func (f *bruschettaFixture) Reset(ctx context.Context) error {
	// Reset the policy in case a test changed it.
	if err := policyutil.ServeAndVerify(ctx, f.fakeDMS, f.chrome, []policy.Policy{f.policy}); err != nil {
		return errors.Wrap(err, "failed to serve bruschetta policy to chrome")
	}

	// Start up the VM, if it's not already running.
	_, err := terminalapp.LaunchBruschetta(ctx, f.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to start bruschetta VM using terminal app")
	}
	if err = apps.Close(ctx, f.tconn, apps.Terminal.ID); err != nil {
		return errors.Wrap(err, "failed to close terminal app")
	}
	// Refresh VM info for new ContextID if the VM was shut down.
	if err := f.concierge.GetVMInfo(ctx, f.vm); err != nil {
		return errors.Wrap(err, "failed to get VM info")
	}

	return nil
}

func (f *bruschettaFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Ensure the display is awake and powered on after potentially long VM setup.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Log("Failed to turn on display: ", err)
	}

	// Keep the display awake to prevent dimming or sleeping during tests.
	cleanup, err := power.RequestKeepAwake(ctx, f.tconn, power.Display)
	if err != nil {
		s.Log("Failed to request keep awake: ", err)
	} else {
		f.releaseKeepAwake = cleanup
	}
}

func (f *bruschettaFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if f.releaseKeepAwake != nil {
		if err := f.releaseKeepAwake(ctx, f.tconn); err != nil {
			s.Log("Failed to release keep awake: ", err)
		}
		f.releaseKeepAwake = nil
	}

	if err := f.saveLogs(ctx, s.OutDir(), "post_test"); err != nil {
		s.Error("Failed to save VM logs from test: ", err)
	}

	// Restart the VM if a test failed. This ensuress all VM windows are closed.
	if s.HasError() {
		if err := f.vm.Stop(ctx); err != nil {
			s.Fatal("Failed to shut down VM: ", err)
		}
	}
}

func (f *bruschettaFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	keepVM, err := strconv.ParseBool(bruschettaKeepVM.Value())
	if err != nil {
		s.Errorf("Failed to parse bruschetta.keep_vm value %q, assuming false: %v", bruschettaKeepVM.Value(), err)
		keepVM = false
	}
	if !keepVM {
		if err := removeBruschetta(ctx, f.tconn); err != nil {
			s.Error("Failed to remove VM after setup failure: ", err)
		}
	}

	if err := f.saveLogs(ctx, s.OutDir(), "tear_down"); err != nil {
		s.Error("Failed to save VM logs from fixture teardown: ", err)
	}

	if f.kb != nil {
		if err := f.kb.Close(ctx); err != nil {
			s.Log("Failure closing keyboard: ", err)
		}
		f.kb = nil
	}
}

func (f *bruschettaFixture) saveLogs(ctx context.Context, outdir, suffix string) error {
	logs, err := f.vm.RetrieveLogs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get VM logs")
	}
	trimmedLogs := logs[f.logOffset:]

	// Set the log offset forward so we skip past this part in future tests.
	f.logOffset = len(logs)

	outFile, err := os.Create(filepath.Join(outdir, fmt.Sprintf("bruschetta_vm_%s.log", suffix)))
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer outFile.Close()

	if _, err := outFile.WriteString(trimmedLogs); err != nil {
		return errors.Wrap(err, "failed to write to log file")
	}

	return nil
}

func (f *bruschettaAppsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	p := s.ParentValue().(FixtureData)
	f.tconn = p.Tconn

	revert, err := devicemode.EnsureDeviceMode(ctx, f.tconn, f.deviceMode)
	if err != nil {
		s.Fatalf("Failed to set device mode to %s: %s", f.deviceMode, err)
	}
	f.revertDeviceMode = revert

	return p
}

func (f *bruschettaAppsFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *bruschettaAppsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *bruschettaAppsFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.revertDeviceMode != nil {
		if err := f.revertDeviceMode(ctx); err != nil {
			s.Log("Failed to reset device mode: ", err)
		}
		f.revertDeviceMode = nil
	}
}

func (f *bruschettaAppsFixture) Reset(ctx context.Context) error {
	return nil
}

func startHTTPServer(ctx context.Context, installerPath, pflashPath string) context.CancelFunc {
	serverCtx, cancel := context.WithCancel(ctx)

	handleFile := func(file string, w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(file)
		if err != nil {
			testing.ContextLogf(serverCtx, "Error: Couldn't open file %q: %v", file, err)
		}
		defer f.Close()
		if _, err := io.Copy(w, f); err != nil {
			testing.ContextLogf(serverCtx, "Error: Couldn't copy file %q to client: %v", file, err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/"+httpInstallerPath, func(w http.ResponseWriter, r *http.Request) {
		// Installer image is compressed using brotli.
		w.Header().Set("Content-Encoding", "br")
		handleFile(installerPath, w, r)
	})
	mux.HandleFunc("/"+httpPflashPath, func(w http.ResponseWriter, r *http.Request) {
		handleFile(pflashPath, w, r)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		testing.ContextLogf(serverCtx, "HTTP server received request to unknown path %s", r.URL)
	})

	server := &http.Server{
		Handler: mux,
		Addr:    httpAddr,
	}

	// Run the server in another goroutine because ListenAndServe() is blocking.
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			testing.ContextLog(serverCtx, "Error starting HTTP server: ", err)
		}
	}()

	// Shutdown the server once the nested context expires.
	go func() {
		<-serverCtx.Done()
		// Note: use ctx here because serverCtx has already expired.
		if err := server.Shutdown(ctx); err != nil {
			testing.ContextLog(ctx, "Error shutting down HTTP server: ", err)
		}
	}()

	return cancel
}

func installBruschetta(ctx context.Context, tconn *chrome.TestConn) error {
	return tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.installBruschetta)`, defaultVMName)
}

func removeBruschetta(ctx context.Context, tconn *chrome.TestConn) error {
	return tconn.Call(ctx, nil, `tast.promisify(chrome.autotestPrivate.removeBruschetta)`, defaultVMName)
}
