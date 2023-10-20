// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	cui "go.chromium.org/tast-tests/cros/local/crostini/ui"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	dlcutil "go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	installationTimeout    = 15 * time.Minute
	checkContainerTimeout  = time.Minute
	takeSnapshotTimeout    = 5 * time.Second
	restoreSnapshotTimeout = 10 * time.Second
	preTestTimeout         = 30 * time.Second
	postTestTimeout        = 30 * time.Second
	uninstallationTimeout  = 2 * time.Minute
	restartCrostiniTimeout = 30*time.Second + terminalapp.LaunchTerminalTimeout

	// snapshotName is the snapshot name for the container.
	snapshotName = "snapshot"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInForCrostini",
		Desc:     "Logged into a session",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := generateChromeOpts(s)
			// Enable ARC++ if it is supported. We do this on every
			// supported device because some tests rely on it and this
			// lets us reduce the number of distinct fixture. If
			// your test relies on ARC++ you should add an appropriate
			// software dependency.
			if arc.Supported() {
				opts = append(opts, chrome.ARCEnabled())
				opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
			} else {
				opts = append(opts, chrome.ARCDisabled())
			}
			return opts, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"keepState"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInWithGaiaForCrostini",
		Desc:     "Logged into a session with Gaia user",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := generateChromeOpts(s)
			if arc.Supported() {
				opts = append(opts, chrome.ARCSupported())
				opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))

			} else {
				opts = append(opts, chrome.ARCDisabled())
			}
			return append(opts, chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))), nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"ui.gaiaPoolDefault", "keepState"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInForCrostiniWithLacros",
		Desc:     "Logged into a session and enable Lacros",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := generateChromeOpts(s)
			if arc.Supported() {
				opts = append(opts, chrome.ARCEnabled())
				opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
			} else {
				opts = append(opts, chrome.ARCDisabled())
			}
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"keepState"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInForCrostiniWithoutArc",
		Desc:     "Logged into a session without Arc enabled",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := generateChromeOpts(s)
			opts = append(opts, chrome.ARCDisabled())
			return opts, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"keepState"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBuster",
		Desc:            "Install Crostini with Buster",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBuster},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostini",

		// TODO (jinrongwu): switch to Global RunTime Variable when deprecating pre.go.
		// The same for the rest keepState var.
		Vars: []string{"keepState"},
		Data: []string{GetContainerMetadataArtifact("buster", false), GetContainerRootfsArtifact("buster", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBullseye",
		Desc:            "Install Crostini with Bullseye",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBullseye},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostini",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", false), GetContainerRootfsArtifact("bullseye", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBookworm",
		Desc:            "Install Crostini with Bookworm",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBookworm},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostini",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bookworm", false), GetContainerRootfsArtifact("bookworm", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBusterGaia",
		Desc:            "Install Crostini with Buster in Chrome logged in with Gaia",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBuster},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInWithGaiaForCrostini",
		Vars:            []string{"keepState", "ui.gaiaPoolDefault"},
		Data:            []string{GetContainerMetadataArtifact("buster", false), GetContainerRootfsArtifact("buster", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBullseyeGaia",
		Desc:            "Install Crostini with Bullseye in Chrome logged in with Gaia",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBullseye},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInWithGaiaForCrostini",
		Vars:            []string{"keepState", "ui.gaiaPoolDefault"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", false), GetContainerRootfsArtifact("bullseye", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBookwormGaia",
		Desc:            "Install Crostini with Bookworm in Chrome logged in with Gaia",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBookworm},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInWithGaiaForCrostini",
		Vars:            []string{"keepState", "ui.gaiaPoolDefault"},
		Data:            []string{GetContainerMetadataArtifact("bookworm", false), GetContainerRootfsArtifact("bookworm", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBullseyeLargeContainer",
		Desc:            "Install Crostini with Bullseye in large container with apps installed",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBullseyeLC},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniWithoutArc",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", true), GetContainerRootfsArtifact("bullseye", true)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBookwormLargeContainer",
		Desc:            "Install Crostini with Bookworm in large container with apps installed",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBookwormLC},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniWithoutArc",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bookworm", true), GetContainerRootfsArtifact("bookworm", true)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBullseyeWithLacros",
		Desc:            "Install Crostini with Bullseye and enable Lacros",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBullseye},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniWithLacros",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", false), GetContainerRootfsArtifact("bullseye", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBookwormWithLacros",
		Desc:            "Install Crostini with Bookworm and enable Lacros",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBookworm},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniWithLacros",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bookworm", false), GetContainerRootfsArtifact("bookworm", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeLoggedInForCrostiniLxdNext",
		Desc:     "Logged into a session (with LXD 5.0)",
		Contacts: []string{"clumptini+oncall@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts := generateChromeOpts(s)
			opts = append(opts, chrome.EnableFeatures("CrostiniUseLxd5"))
			return opts, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"keepState"},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBullseyeWithLxdNext",
		Desc:            "Install Crostini with Bullseye and LXD 5.0",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBullseye},
		SetUpTimeout:    installationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniLxdNext",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", false), GetContainerRootfsArtifact("bullseye", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:            "crostiniBookwormWithLxdNext",
		Desc:            "Install Crostini with Bookworm and LXD 5.0",
		Contacts:        []string{"clumptini+oncall@google.com"},
		Impl:            &crostiniFixture{preData: preTestDataBookworm},
		SetUpTimeout:    installationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          "chromeLoggedInForCrostiniLxdNext",
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bookworm", false), GetContainerRootfsArtifact("bookworm", false)},
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "crostiniBullseyePolicy",
		Desc:     "Install Crostini with Bullseye, with Chrome logged in with policy",
		Contacts: []string{"clumptini+oncall@google.com", "aidazolic@google.com"},
		Impl: &crostiniFixture{preData: preTestDataBullseye,
			extraOptsFunc: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{chrome.EnableFeatures("NewFilesPolicyUX")}, nil
			},
		},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    checkContainerTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Parent:          fixture.ChromePolicyLoggedIn,
		Vars:            []string{"keepState"},
		Data:            []string{GetContainerMetadataArtifact("bullseye", false), GetContainerRootfsArtifact("bullseye", false)},
	})
}

// preTestData contains the data to set up the fixture.
type preTestData struct {
	container     containerType
	debianVersion vm.ContainerDebianVersion
	startedOK     bool
}

// crostiniFixture holds the runtime state of the fixture.
type crostiniFixture struct {
	cr            *chrome.Chrome
	tconn         *chrome.TestConn
	cont          *vm.Container
	kb            *input.KeyboardEventWriter
	preData       *preTestData
	postData      *PostTestData
	values        *perf.Values
	logDir        string
	extraOptsFunc chrome.OptionsCallback
}

// FixtureData is the data returned by SetUp and passed to tests.
type FixtureData struct {
	Chrome        *chrome.Chrome
	Tconn         *chrome.TestConn
	Cont          *vm.Container
	KB            *input.KeyboardEventWriter
	PostData      *PostTestData
	StartupValues *perf.Values
	Screendiffer  *Screendiffer
	DownloadsPath string
	FakeDMS       *fakedms.FakeDMS
}

var preTestDataBuster = &preTestData{
	container:     normal,
	debianVersion: vm.DebianBuster,
}

var preTestDataBullseye = &preTestData{
	container:     normal,
	debianVersion: vm.DebianBullseye,
}

var preTestDataBookworm = &preTestData{
	container:     normal,
	debianVersion: vm.DebianBookworm,
}

var preTestDataBullseyeLC = &preTestData{
	container:     largeContainer,
	debianVersion: vm.DebianBullseye,
}

var preTestDataBookwormLC = &preTestData{
	container:     largeContainer,
	debianVersion: vm.DebianBookworm,
}

// Differ returns an instance implementing the interface screenshot.Differ.
func (f FixtureData) Differ() screenshot.Differ {
	if f.Screendiffer != nil {
		return *f.Screendiffer.differ
	}
	return nil
}

func (f *crostiniFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.postData = &PostTestData{}
	f.cr = s.ParentValue().(chrome.HasChrome).Chrome()
	f.logDir = s.OutDir()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, uninstallationTimeout)
	defer cancel()

	// If initialization fails, this defer is used to clean-up the partially-initialized pre
	// and copies over lxc + container boot logs.
	// Stolen verbatim from arc/pre.go
	shouldClose := true
	defer func() {
		if shouldClose {
			// TODO (jinrongwu): use FixtureData instead of PreData and modify RunCrostiniPostTest when deprecating pre.go.
			RunCrostiniPostTest(cleanupCtx, PreData{f.cr, f.tconn, f.cont, f.kb, f.postData})
			f.cleanUp(cleanupCtx, s)
		}
	}()

	// To help identify sources of flake, we report disk usage before the test.
	if err := reportDiskUsage(ctx); err != nil {
		s.Log("Failed to gather disk usage: ", err)
	}

	var err error
	if f.tconn, err = f.cr.TestAPIConn(ctx); err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	ownerID, err := cryptohome.UserHash(ctx, f.cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get owner ID: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, f.tconn)

	if f.kb, err = input.Keyboard(ctx); err != nil {
		s.Fatal("Failed to create keyboard device: ", err)
	}

	// Setup the screen recorder.
	screenRecorder := uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)
	hasChromeBeenReset := false
	defer func(ctx context.Context) {
		// The recorder will not exist if Chrome has been reset.
		if !hasChromeBeenReset {
			screenRecorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
		}
	}(cleanupCtx)

	// Setup the perf recorder.
	perfRecorder, err := StartRecording(ctx, f.tconn, "crostini_restart", RestartStages)
	if err != nil {
		s.Log("Can't record initial restart metrics: ", err)
	}

	skippedInstall := false
	if checkKeepState(s) && terminaDiskExists(ownerID) {
		s.Log("keepState attempting to start the existing VM and container by launching Terminal")
		if err = f.launchExitTerminal(ctx); err != nil {
			s.Fatal("KeepState error: ", err)
		}
		skippedInstall = true
	} else {
		// Install Crostini.
		iOptions := GetInstallerOptions(s, f.preData.debianVersion, f.preData.container == largeContainer, f.cr.NormalizedUser())
		if _, err := cui.InstallCrostini(ctx, f.tconn, f.cr, iOptions); err != nil {
			// Try to retrieve crostini_journalctl if the container is starting
			// but the installation fails.
			if termina, err := vm.GetRunningVM(ctx, f.cr.NormalizedUser(), vm.Termina); err != nil {
				s.Log("Cannot get running VM: ", err)
			} else if termina != nil {
				termina.TrySaveContainerLogs(ctx, s.OutDir())
			}
			s.Fatal("Failed to install Crostini: ", err)
		}
	}
	if f.values, err = perfRecorder.UpdateValues(ctx, f.tconn); err != nil {
		s.Log("Can't update perf values: ", err)
	} else {
		f.values.Save(s.OutDir())
	}

	f.cont, err = vm.DefaultContainer(ctx, f.cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to connect to running container: ", err)
	}

	// Report disk size again after successful install.
	if err := reportDiskUsage(ctx); err != nil {
		s.Log("Failed to gather disk usage: ", err)
	}

	if err := verifyAutoUpdatesDisabled(ctx, f.cont); err != nil {
		s.Fatal("Failed to verify automatic updates are disabled: ", err)
	}

	if skippedInstall {
		if err := f.cont.RestoreSnapshot(ctx, snapshotName, f.logDir); err != nil {
			s.Fatal("KeepState failed to restore snapshot: ", err)
		}
	} else {
		if err := f.cont.CreateSnapshot(ctx, snapshotName, s.OutDir()); err != nil {
			s.Fatal("Failed to take snapshot before test: ", err)
		}
	}

	// Launching Terminal after restart container by lxc is needed to
	// ensure a bunch of things work, e.g., mouting files in FilesApp.
	// See b/271947202.
	if err := f.launchExitTerminal(ctx); err != nil {
		s.Fatal("Failed to re-launch terminal and exit after creating snapshot: ", err)
	}

	// If the wayland backend is used, the fonctconfig cache will be
	// generated the first time the app starts. On a low-end device, this
	// can take a long time and timeout the app executions below.
	testing.ContextLog(ctx, "Generating fontconfig cache")
	if err := f.cont.Command(ctx, "fc-cache").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to generate fontconfig cache: ", err)
	}

	// Disable cursor blinking for GTK apps.
	if err := guestos.DisableCursorBlinking(ctx, f.cont); err != nil {
		s.Fatal("Failed to stop cursor blink: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, f.cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Downloads path: ", err)
	}

	if err := f.cr.ResetState(ctx); err != nil {
		s.Fatal("Failed to reset chrome's state: ", err)
	}
	hasChromeBeenReset = true

	f.preData.startedOK = true
	vm.Lock()
	shouldClose = false

	var fakeDMS *fakedms.FakeDMS
	hasFakeDMS, ok := s.ParentValue().(fakedms.HasFakeDMS)
	if ok {
		fakeDMS = hasFakeDMS.FakeDMS()
	}

	return FixtureData{
		Chrome:        f.cr,
		Tconn:         f.tconn,
		Cont:          f.cont,
		KB:            f.kb,
		PostData:      f.postData,
		StartupValues: f.values,
		Screendiffer:  nil,
		DownloadsPath: downloadsPath,
		FakeDMS:       fakeDMS,
	}
}

func (f *crostiniFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	f.close(ctx, s)
}

func (f *crostiniFixture) Reset(ctx context.Context) error {
	resetSucceeds := false
	defer func() {
		f.preData.startedOK = resetSucceeds
	}()
	// Check container.
	// It returns error in the following situations:
	// 1. no container
	// 2. container does not work
	// 3. the container snapshot could not be restored
	// 4. chrome is not responsive
	// 5. fail to reset chrome.
	// Note that 4 and 5 is already done by the parent fixture.
	// Otherwise, return nil.
	if f.cont == nil {
		return errors.New("There is no container")
	}
	if err := f.cont.Connect(ctx, f.cr.NormalizedUser()); err != nil {
		return errors.Wrap(err, "failed to reconnect to the running VM")
	}

	// 1. stop the container.
	// 2. restore the snapshot.
	// 3. start the container.
	if err := f.cont.RestoreSnapshot(ctx, snapshotName, f.logDir); err != nil {
		return errors.Wrap(err, "failed to restore snapshot")
	}
	// Launching Terminal after storing snapshot by lxc is needed to ensure
	// a bunch of things work, e.g., mouting files in FilesApp.
	// See b/271947202.
	if err := f.launchExitTerminal(ctx); err != nil {
		return errors.Wrap(err, "failed to re-launch terminal and exit")
	}

	if err := BasicCommandWorks(ctx, f.cont); err != nil {
		return errors.Wrap(err, "failed to check basic commands in the existing container")
	}

	resetSucceeds = true
	return nil
}

func (f *crostiniFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *crostiniFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// TODO (jinrongwu): use FixtureData instead of PreData and modify RunCrostiniPostTest when deprecating pre.go.
	RunCrostiniPostTest(ctx, PreData{f.cr, f.tconn, f.cont, f.kb, f.postData})
}

func (f *crostiniFixture) close(ctx context.Context, s *testing.FixtState) {
	vm.Unlock()
	f.cleanUp(ctx, s)
}

// cleanUp de-initializes the fixture by closing/cleaning-up the relevant
// fields and resetting the struct's fields.
func (f *crostiniFixture) cleanUp(ctx context.Context, s *testing.FixtState) {
	if f.kb != nil {
		if err := f.kb.Close(ctx); err != nil {
			s.Log("Failure closing keyboard: ", err)
		}
		f.kb = nil
	}

	if f.postData.vmLogReader != nil {
		if err := f.postData.vmLogReader.Close(); err != nil {
			s.Log("Failed to close VM log reader: ", err)
		}
	}

	// Don't uninstall crostini or delete the image for keepState so that
	// crostini is still running after the test, and the image can be reused.
	if checkKeepState(s) && f.preData.startedOK {
		s.Log("keepState not uninstalling Crostini and deleting image in cleanUp")
	} else {
		if f.cont != nil {
			if err := uninstallLinuxFromUI(ctx, f.tconn, f.cr); err != nil {
				s.Log("Failed to uninstall Linux: ", err)
			}
			f.cont = nil
		}

		// Unmount the VM image to prevent later tests from
		// using it by accident. Otherwise we may have a dlc
		// test use the component or vice versa.
		if err := dlcutil.Uninstall(ctx, "termina-dlc"); err != nil {
			s.Error("Failed to unmount termina-dlc: ", err)
		}

		if err := vm.DeleteImages(); err != nil {
			s.Log("Error deleting images: ", err)
		}
	}
	f.preData.startedOK = false

	// Nothing special needs to be done to close the test API connection.
	f.tconn = nil

	f.cr = nil
}

func (f *crostiniFixture) launchExitTerminal(ctx context.Context) error {
	_, err := terminalapp.Launch(ctx, f.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch Terminal")
	}
	if err = apps.Close(ctx, f.tconn, apps.Terminal.ID); err != nil {
		return errors.Wrap(err, "failed to exit Terminal window")
	}
	return nil
}

// checkKeepState returns whether the fixture should keep state from the
// previous test execution and try to recycle the VM.
func checkKeepState(s *testing.FixtState) bool {
	if str, ok := s.Var("keepState"); ok {
		b, err := strconv.ParseBool(str)
		if err != nil {
			s.Fatalf("Cannot parse argument %q to keepState: %v", str, err)
		}
		return b
	}
	return false
}

// terminaDiskExists returns true if the termina disk exists.
func terminaDiskExists(ownerID string) bool {
	_, err := os.Stat(fmt.Sprintf("/run/daemon-store/crosvm/%s/dGVybWluYQ==.img", ownerID))
	return err == nil
}

// generateChromeOpts generates common chrome options for crostini fixtures.
func generateChromeOpts(s *testing.FixtState) []chrome.Option {
	opts := []chrome.Option{chrome.ExtraArgs("--vmodule=crostini*=1")}

	useLocalImage := checkKeepState(s)
	if useLocalImage {
		// Retain the user's cryptohome directory and previously installed VM.
		opts = append(opts, chrome.KeepState())
	}

	return opts
}

func verifyAutoUpdatesDisabled(ctx context.Context, cont *vm.Container) error {
	for _, unit := range []string{"apt-daily.timer", "apt-daily-upgrade.timer"} {
		if err := cont.Command(ctx, "sh", "-c", fmt.Sprintf("! systemctl is-active %s", unit)).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "error checking %q", unit)
		}
	}

	if err := cont.Command(ctx, "grep", "-q", "^DisableAutomaticCrosPackageUpdates=true$", ".config/cros-garcon.conf").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "error checking automatic cros package updates")
	}

	if err := cont.Command(ctx, "grep", "-q", "^DisableAutomaticSecurityUpdates=true$", ".config/cros-garcon.conf").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "error checking automatic security updates")
	}

	return nil
}
