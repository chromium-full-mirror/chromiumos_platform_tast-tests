// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package saveddesks

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ResetTimeout is the timeout duration to try to reset of the current precondition.
const ResetTimeout = 30 * time.Second

// PreTestTimeout is the timeout duration to reset output directory before each test.
const PreTestTimeout = 15 * time.Second

// PostTestTimeout is the timeout duration to save logs after each test.
// It's intentionally set longer than ResetTimeout because dumping 'dumpsys' takes around 20 seconds.
const PostTestTimeout = ResetTimeout + 20*time.Second

// UserDataDir is the directory that contains the user data of saveddesks.
const UserDataDir = "/home/chronos/user/saveddesks/"

func init() {
	// savedDesksEnableWithoutArc is a fixture that enables saved desks functionality without ARC.
	testing.AddFixture(&testing.Fixture{
		Name: "savedDesksEnableWithoutArc",
		Desc: "Saved desks features enabled without ARC",
		Vars: []string{ui.GaiaPoolDefaultVarName},
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		BugComponent: "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl: bootedWithFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
				chrome.EnableFeatures("DesksTemplates", "EnableSavedDesks"),
				chrome.DisableFeatures("DeskTemplateSync"),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// savedDesksEnableWithArc is a fixture that enables saved desks functionality alongside ARC.
	testing.AddFixture(&testing.Fixture{
		Name: "savedDesksEnableWithArc",
		Desc: "Saved desks features enabled with ARC",
		Vars: []string{ui.GaiaPoolDefaultVarName},
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		BugComponent: "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl: bootedWithARCFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
				chrome.EnableFeatures("DesksTemplates", "EnableSavedDesks"),
				chrome.DisableFeatures("DeskTemplateSync"),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// savedDesksCUJEnableWithoutArc is a fixture that enables saved desks CUJ functionality.
	testing.AddFixture(&testing.Fixture{
		Name: "savedDesksCUJEnableWithoutArc",
		Desc: "Saved desks features enabled without ARC",
		Vars: []string{ui.GaiaPoolDefaultVarName},
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		BugComponent: "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl: bootedWithFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
				chrome.EnableFeatures("DesksTemplates", "EnableSavedDesks"),
				chrome.DisableFeatures("DeskTemplateSync", "FirmwareUpdaterApp"),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// savedDesksCUJEnableWithArc is a fixture that enables saved desks CUJ functionality alongside ARC.
	testing.AddFixture(&testing.Fixture{
		Name: "savedDesksCUJEnableWithArc",
		Desc: "Saved desks features enabled with ARC",
		Vars: []string{ui.GaiaPoolDefaultVarName},
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		BugComponent: "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl: bootedWithARCFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
				chrome.EnableFeatures("DesksTemplates", "EnableSavedDesks"),
				chrome.DisableFeatures("DeskTemplateSync", "FirmwareUpdaterApp"),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})

	// savedDesksEnabledLacrosWithArcBooted is a fixture that combines the functionality of ARC,
	// lacros, and saved desks.
	testing.AddFixture(&testing.Fixture{
		Name: "savedDesksEnabledLacrosWithArcBooted",
		Desc: "Saved desks features enabled with lacros and ARC",
		Vars: []string{ui.GaiaPoolDefaultVarName},
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"zhumatthew@google.com",
		},
		BugComponent: "b:1020793", // ChromeOS Server Projects > Enterprise Management > Commercial Productivity
		Impl: bootedWithARCFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)), chrome.EnableFeatures("DesksTemplates", "EnableSavedDesks"),
				chrome.DisableFeatures("DeskTemplateSync"))).Opts()
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout + optin.OptinTimeout + arc.BootTimeout + 2*time.Minute,
		ResetTimeout:    ResetTimeout,
		PostTestTimeout: PostTestTimeout,
		TearDownTimeout: ResetTimeout,
	})
}

type savedDesksFixture struct {
	cr           *chrome.Chrome
	arc          *arc.ARC
	initSnapshot *arc.Snapshot

	arcEnabled bool // ARC is enabled.

	fOpt chrome.OptionsCallback // Function to return chrome options.
}

// SavedDeskFixtData is returned by the fixtures and used in tests.
type SavedDeskFixtData struct {
	// Chrome is a connection to an already-started Chrome instance.
	// It cannot be closed by tests.
	Chrome *chrome.Chrome
}

// bootedWithFixture returns a FixtureImpl with a OptionsCallback function provided.
func bootedWithFixture(fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &savedDesksFixture{
		fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := fOpts(ctx, s)
			if err != nil {
				return nil, err
			}
			return opts, nil
		},
	}
}

// bootedWithARCFixture returns a FixtureImpl with a OptionsCallback function provided.
// ARCSupported() will always be added to the Chrome options returned by OptionsCallback.
func bootedWithARCFixture(fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &savedDesksFixture{
		arcEnabled: true,
		fOpt: func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := fOpts(ctx, s)
			if err != nil {
				return nil, err
			}
			return append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...), chrome.ARCSupported()), nil
		},
	}
}

func (f *savedDesksFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	success := false
	f.arc = nil
	f.initSnapshot = nil
	opts, err := f.fOpt(ctx, s)
	if err != nil {
		s.Fatal("Failed to obtain fixture options: ", err)
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if !success {
			cr.Close(ctx)
		}
	}()
	if f.arcEnabled {
		s.Log("Performing Play Store Optin")
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to connect Test API: ", err)
		}
		st, err := arc.GetState(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get ARC state: ", err)
		}
		if st.Provisioned {
			s.Log("ARC is already provisioned. Skipping the Play Store setup")
		} else {
			// Opt into Play Store and close the Play Store window.
			if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
				s.Fatal("Failed to opt into Play Store: ", err)
			}
		}

		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer func() {
			if !success {
				a.Close(ctx)
			}
		}()

		initSnapshot, err := arc.NewSnapshot(ctx, a)
		if err != nil {
			s.Fatal("Failed to take ARC state snapshot: ", err)
		}
		f.initSnapshot = initSnapshot

		// Prevent the arc package's New and Close functions from being called
		// while this savedDesksFixture is active.
		arc.Lock()
		f.arc = a
	}
	// Prevent the chrome package's New and Close functions from being called
	// while this savedDesksFixture is active.
	chrome.Lock()
	f.cr = cr

	success = true

	return &SavedDeskFixtData{
		Chrome: cr,
	}
}

func (f *savedDesksFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.arcEnabled {
		arc.Unlock()
		if err := f.arc.Close(ctx); err != nil {
			s.Log("Failed to close ARC: ", err)
		}
		f.arc = nil
	}

	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}
	f.cr = nil
}

func (f *savedDesksFixture) Reset(ctx context.Context) error {
	if err := os.RemoveAll(UserDataDir); err != nil {
		return errors.Wrap(err, "failed to delete saveddesks user data directory")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed to reset chrome")
	}
	if f.arcEnabled {
		return f.initSnapshot.Restore(ctx, f.arc)
	}
	return nil
}

func (f *savedDesksFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	if f.arcEnabled {
		if err := f.arc.ResetOutDir(ctx, s.OutDir()); err != nil {
			s.Error("Failed to to reset outDir field of ARC object: ", err)
		}
	}
}

func (f *savedDesksFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	faillogDir := filepath.Join(s.OutDir(), "faillog")
	if s.HasError() {
		if err := os.MkdirAll(faillogDir, 0755); err != nil {
			s.Error("Failed to make faillog/ directory: ", err)
			return
		}
		if f.arcEnabled {
			if err := f.arc.SaveLogFiles(ctx); err != nil {
				s.Error("Failed to to save ARC-related log files: ", err)
			}
			if err := saveProcessList(ctx, f.arc, faillogDir); err != nil {
				s.Error("Failed to save the process list in ARCVM: ", err)
			}
			if err := saveDumpsys(ctx, f.arc, faillogDir); err != nil {
				s.Error("Failed to save dumpsys output in ARCVM: ", err)
			}
		}
	}
}

func saveProcessList(ctx context.Context, a *arc.ARC, outDir string) error {
	path := filepath.Join(outDir, "ps-arcvm.txt")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "ps", "-AfZ")
	cmd.Stdout = file
	return cmd.Run()
}

func saveDumpsys(ctx context.Context, a *arc.ARC, outDir string) error {
	path := filepath.Join(outDir, "dumpsys.txt")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := a.Command(ctx, "dumpsys")
	cmd.Stdout = file
	return cmd.Run()
}
