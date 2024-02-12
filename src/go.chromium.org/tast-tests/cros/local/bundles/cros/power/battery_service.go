// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

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

// PrepareBattery drains or charges the battery to ensure the it is within the specified charge percentage.
func (b *BatteryService) PrepareBattery(ctx context.Context, req *power.BatteryRequest) (*empty.Empty, error) {
	chargeParams := pow.ChargeParams{
		MinChargePercentage:   float64(req.MinPercentage),
		MaxChargePercentage:   float64(req.MaxPercentage),
		DischargeOnCompletion: req.DischargeOnCompletion,
	}
	if err := setup.PrepareBattery(ctx, chargeParams); err != nil {
		return nil, errors.Wrapf(err, "failed to put the battery within [%.2f%%, %.2f%%]", req.MinPercentage, req.MaxPercentage)
	}
	return &empty.Empty{}, nil
}

func (b *BatteryService) StopChargeLimit(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	if !setup.ChargeLimitEnabled(ctx) || !setup.ChargeControlV2Support(ctx) {
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

func (b *BatteryService) DisableBatteryCharging(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, setup.DisableBatteryCharging(ctx)
}

func (b *BatteryService) AllowBatteryCharging(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return &empty.Empty{}, setup.AllowBatteryCharging(ctx)
}
