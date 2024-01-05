// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TitleBar,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test the Title Bar of the ARC App and Its buttons",
		Contacts:     []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Timeout:      arc.BootTimeout + 2*time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraAttr:         []string{"group:hw_agnostic"},
		}},
	})
}

func TitleBar(ctx context.Context, s *testing.State) {
	const (
		apk     = "ArcAppValidityTest.apk"
		pkgName = "org.chromium.arc.testapp.appvaliditytast"
		cls     = ".MainActivity"
	)

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to enter clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	act, err := arc.NewActivity(a, pkgName, cls)
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	defer act.Close(ctx)

	s.Log("Starting app")
	if err = act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start app: ", err)
	}
	defer act.Stop(ctx, tconn)

	wanted := ash.CaptionButtonBack | ash.CaptionButtonMinimize | ash.CaptionButtonMaximizeAndRestore | ash.CaptionButtonClose

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Get activity's window info.
		info, err := ash.GetARCAppWindowInfo(ctx, tconn, pkgName)
		if err != nil {
			return errors.Wrapf(err, "failed to ARC window infomation for package name %s", pkgName)
		}

		if info.CaptionButtonEnabledStatus != wanted {
			return errors.Errorf("Wanted %s got %s", wanted.String(), info.CaptionButtonEnabledStatus.String())
		}
		return nil

	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Caption button check failed : ", err)
	}

	ui := uiauto.New(tconn)
	if t, ok := arc.Type(); ok && t == arc.VM {
		s.Log("ARC-R")
		if err := uiauto.Combine("maximize and restore it back",
			ui.LeftClick(nodewith.Name("Maximize").Role(role.Button)),
			ui.LeftClick(nodewith.Name("Restore").Role(role.Button)),
		)(ctx); err != nil {
			s.Fatal("Failed to Maximize and Restore it back : ", err)
		}
	} else if ok && t == arc.Container {
		s.Log("ARC-P")
		if err := uiauto.Combine("restore and maximize it back",
			ui.LeftClick(nodewith.Name("Restore").Role(role.Button)),
			ui.LeftClick(nodewith.Name("Maximize").Role(role.Button)),
		)(ctx); err != nil {
			s.Fatal("Failed to Restore and Maximize it back : ", err)
		}
	} else {
		s.Errorf("Unsupported ARC type %d", t)
	}

	s.Log("Press Back Buttton")
	if err := d.PressKeyCode(ctx, androidui.KEYCODE_BACK, 0); err != nil {
		s.Fatal("Failed to enter KEYCODE_BACK: ", err)
	}

	s.Log("Restart activity")
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed start activity: ", err)
	}

	s.Log("Tap on Close ")
	if err := ui.LeftClick(nodewith.Name("Close").Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to Close: ", err)
	}

}
