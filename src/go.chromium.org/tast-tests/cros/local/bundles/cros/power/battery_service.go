// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	pow "go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast-tests/cros/services/cros/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			power.RegisterBatteryServiceServer(srv, &BatteryService{s: s})
		},
	})
}

// BatteryService implements tast.cros.power.BatteryService.
type BatteryService struct {
	s       *testing.ServiceState
	cr      *chrome.Chrome
	prefs   *setup.TmpPrefs
	cleanup setup.CleanupCallback
}

// New logs into a Chrome session as a fake user. Close must be called later
// to clean up the associated resources.
func (b *BatteryService) New(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if b.cr != nil {
		return nil, errors.New("Chrome already available")
	}

	cr, err := chrome.New(ctx)
	if err != nil {
		return nil, err
	}
	b.cr = cr

	prefs := &setup.TmpPrefs{}
	cl, err := prefs.InitTmpPrefs(ctx)
	if err != nil {
		return nil, err
	}

	b.prefs = prefs
	b.cleanup = cl
	return &empty.Empty{}, nil
}

// Close releases the resources obtained by New.
func (b *BatteryService) Close(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	var err error
	if b.cr != nil {
		if err = b.cr.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close Chrome with error: ", err)
		}
	}
	b.cr = nil
	if b.cleanup != nil {
		if err2 := b.cleanup(ctx); err2 != nil {
			testing.ContextLog(ctx, "Failed to cleanup temporary prefs with error: ", err)
			err = err2
		}
	}
	b.cleanup = nil
	return &empty.Empty{}, err
}

// DrainBattery drains the battery to ensure the device battery is within the specified charge percentage.
func (b *BatteryService) DrainBattery(ctx context.Context, req *power.BatteryRequest) (*empty.Empty, error) {
	if b.cr == nil {
		return nil, errors.New("Chrome not available")
	}
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Maxing out screen brightness to drain faster.
	pm, err := pow.NewPowerManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a PowerManager object")
	}
	brightness, err := pm.GetScreenBrightnessPercent(ctx)
	if err := pm.SetScreenBrightness(ctx, 100); err != nil {
		return nil, errors.Wrap(err, "failed to update screen brightness")
	}
	defer func(ctx context.Context) {
		if err := pm.SetScreenBrightness(ctx, brightness); err != nil {
			testing.ContextLogf(ctx, "Failed to reset screen brightness to %.2f%%: %v", brightness, err)
		}
	}(cleanupCtx)

	// Rendering a WebGL website to consume power quickly.
	conn, err := b.cr.NewConn(ctx, "https://crospower.page.link/power_BatteryDrain")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if err := testing.Poll(ctx, func(context.Context) error {
		status, err := pow.GetStatus(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to obtain DUT power status"))
		}
		if status.LinePowerConnected {
			return testing.PollBreak(errors.New("battery draining requires device disconnected from the power source"))
		}
		if float32(status.BatteryPercent) > req.MaxPercentage {
			return errors.Errorf("still discharging from %.2f%% to %.2f%%", status.BatteryPercent, req.MaxPercentage)
		}
		return nil
	}, &testing.PollOptions{
		Interval: time.Minute,
		Timeout:  45 * time.Minute,
	}); err != nil {
		return nil, errors.Wrapf(err, "failed to drain battery to %.2f%%", req.MaxPercentage)
	}
	return &empty.Empty{}, nil
}

func (b *BatteryService) StopChargeLimit(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if !setup.ChargeLimitEnabled(ctx) {
		return &empty.Empty{}, nil
	}
	if _, err := setup.StopChargeLimit(ctx, b.prefs); err != nil {
		return nil, errors.Wrap(err, "failed to stop Charge Limit")
	}
	return &empty.Empty{}, nil
}

// PowerSettingInIdleMode provides power settings required during IDLE mode.
func (b *BatteryService) PowerSettingInIdleMode(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if b.cr == nil {
		return nil, errors.New("Chrome not available")
	}

	tconn, err := b.cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Test API connection")
	}

	ui := uiauto.New(tconn)
	settings, err := ossettings.LaunchAtPage(ctx, tconn, nodewith.Name("Power").Role(role.Link))
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch os-settings Power page")
	}
	defer settings.Close(ctx)

	idleActionWhileCharging := nodewith.Name("Idle action while charging").Role(role.ComboBoxSelect)
	if err := ui.LeftClick(idleActionWhileCharging)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to left click on idle action while charging in combo box")
	}

	keepDisplayOnListBox := nodewith.Name("Keep display on").Role(role.ListBoxOption)
	if err := ui.LeftClick(keepDisplayOnListBox)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to left click on keep display in list box")
	}

	idleActionOnBattery := nodewith.Name("Idle action while on battery").Role(role.ComboBoxSelect)
	if err := ui.LeftClick(idleActionOnBattery)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to left click on idle action while on battery in combo box")
	}

	if err := ui.LeftClick(keepDisplayOnListBox)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to left click on keep display in list box")
	}
	return &empty.Empty{}, nil
}
