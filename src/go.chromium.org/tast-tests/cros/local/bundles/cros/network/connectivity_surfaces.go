// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const osSettingsSurfaceName = "OS Settings"
const quickSettingsSurfaceName = "Quick Settings"

type connectivitySurfacesTestCase struct {
	// name is the name of the surface.
	name string
	// openSurface is a function that opens the surface and returns a cleanup function.
	openSurface func(context.Context, *chrome.TestConn) (func(context.Context), error)
	// root is the finder for the root node of the surface.
	root *nodewith.Finder
	// finders are the finders for nodes that are expected to be found.
	finders []*nodewith.Finder
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ConnectivitySurfaces,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "ConnectivitySurfaces is a test that is intended to be run during CQ to prevent regressions resulting from changes to ChromeOS Connectivity surfaces",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chadduffin@google.com",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{
			{
				Name: "os_settings__bluetooth",
				Val: &connectivitySurfacesTestCase{
					name: osSettingsSurfaceName,
					openSurface: func(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
						return showOSSettingsPage(ctx, tconn, ossettings.Bluetooth)
					},
					root: ossettings.WindowFinder,
					finders: []*nodewith.Finder{
						ossettings.OSSettingsBluetoothToggleButton,
						ossettings.BluetoothPairNewDeviceButton,
					},
				},
			},
			{
				Name:              "os_settings__cellular",
				ExtraAttr:         []string{"group:cellular", "cellular_unstable"},
				ExtraHardwareDeps: hwdep.D(hwdep.Cellular()),
				ExtraTestBedDeps:  []string{tbdep.CellularModemState("NORMAL")},
				Val: &connectivitySurfacesTestCase{
					name: osSettingsSurfaceName,
					openSurface: func(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
						return showOSSettingsPage(ctx, tconn, ossettings.Network)
					},
					root: ossettings.WindowFinder,
					finders: []*nodewith.Finder{
						ossettings.MobileButton,
					},
				},
			},
			{
				Name: "os_settings__network",
				Val: &connectivitySurfacesTestCase{
					name: osSettingsSurfaceName,
					openSurface: func(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
						return showOSSettingsPage(ctx, tconn, ossettings.Network)
					},
					root: ossettings.WindowFinder,
					finders: []*nodewith.Finder{
						ossettings.AddConnectionButton,
					},
				},
			},
			{
				Name: "os_settings__wifi",
				Val: &connectivitySurfacesTestCase{
					name: osSettingsSurfaceName,
					openSurface: func(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
						return showOSSettingsPage(ctx, tconn, ossettings.Network)
					},
					root: ossettings.WindowFinder,
					finders: []*nodewith.Finder{
						ossettings.WifiButton,
					},
				},
			},
			{
				Name: "quick_settings__bluetooth",
				Val: &connectivitySurfacesTestCase{
					name:        quickSettingsSurfaceName,
					openSurface: showQuickSettings,
					root:        quicksettings.QsRootFinder,
					finders: []*nodewith.Finder{
						quicksettings.FeatureTileBluetooth,
						quicksettings.FeatureTileBluetoothToggle,
					},
				},
			},
			{
				Name: "quick_settings__network",
				Val: &connectivitySurfacesTestCase{
					name:        quickSettingsSurfaceName,
					openSurface: showQuickSettings,
					root:        quicksettings.QsRootFinder,
					finders: []*nodewith.Finder{
						quicksettings.FeatureTileNetwork,
					},
				},
			},
		},
	})
}

// ConnectivitySurfaces is a test that is intended to be run during CQ to prevent regressions resulting from changes to ChromeOS Connectivity surfaces.
func ConnectivitySurfaces(ctx context.Context, s *testing.State) {
	testCase := s.Param().(*connectivitySurfacesTestCase)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupFunc, err := testCase.openSurface(ctx, tconn)
	if err != nil {
		s.Fatalf("Failed to open the %q surface: %v", testCase.name, err)
	}
	defer cleanupFunc(cleanupCtx)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	// Check that the expected UI node exists over some amount of time.
	sampleInterval := 5
	sampleTimeout := 10
	sampleCount := sampleTimeout / sampleInterval

	for _, finder := range testCase.finders {
		verifyNodeExists := ui.WaitUntilExists(finder.Ancestor(testCase.root))

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			testing.ContextLogf(ctx, "Searching for %q", finder.Pretty())

			if err := verifyNodeExists(ctx); err != nil {
				return testing.PollBreak(err)
			}

			sample := 0

			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err = verifyNodeExists(ctx); err != nil {
					return err
				}
				testing.ContextLogf(ctx, "Found %d of %d times", sample+1, sampleCount)

				sample++
				if sample < sampleCount {
					return errors.New("continuing to sample")
				}
				return nil
			}, &testing.PollOptions{
				Timeout:  time.Second * time.Duration(sampleTimeout),
				Interval: time.Second * time.Duration(sampleInterval),
			}); err != nil {
				return errors.Wrapf(err, "failed to find %q", finder.Pretty())
			}
			return nil
		}, &testing.PollOptions{Timeout: time.Second * 10, Interval: time.Second}); err != nil {
			s.Fatalf("Failed to find node in the %q surface: %v", testCase.name, err)
		}
	}
}

func showQuickSettings(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to show Quick Settings")
	}
	return func(cleanupCtx context.Context) {
		quicksettings.Hide(cleanupCtx, tconn)
	}, nil
}

func showOSSettingsPage(ctx context.Context, tconn *chrome.TestConn, page *nodewith.Finder) (func(context.Context), error) {
	app, err := ossettings.LaunchAtPage(ctx, tconn, page)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to show %q page of OS Settings", page.Pretty())
	}
	return func(cleanupCtx context.Context) {
		app.Close(cleanupCtx)
	}, nil
}
