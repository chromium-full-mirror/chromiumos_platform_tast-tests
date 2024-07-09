// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dma

import (
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/accountmanager"
	"go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/arcappcompat"
	"go.chromium.org/tast-tests/cros/common/assistant"
	"go.chromium.org/tast-tests/cros/common/calendar"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/connectivityfwdumps"
	"go.chromium.org/tast-tests/cros/common/dev"
	"go.chromium.org/tast-tests/cros/common/drivefs"
	"go.chromium.org/tast-tests/cros/common/enterpriseconnectors"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/common/filemanager"
	"go.chromium.org/tast-tests/cros/common/floatingworkspace"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/common/wallpaper"
	"go.chromium.org/tast/core/testing"
)

var dmaEnableVar = testing.RegisterVarString(
	"dma.Enable",
	"false",
	"It indicates whether dma is enabled or not",
)

func pools() (map[string]string, map[string]string) {
	var dmaPools = map[string]string{
		accountmanager.AccountPoolVarName:           ui.GaiaDMAPoolDefaultValue(),
		arcappcompat.AccountVarName:                 arcappcompat.DmaAccountValue(),
		arc.ChildAccountVarName:                     arc.ChildDMAAccountValue(),
		arc.DrivefsPoolVarName:                      ui.GaiaDMAPoolDefaultValue(),
		arc.Managed3pEmmAccountVarName:              arc.ManagedDMAAccountPoolValue(),
		arc.ManagedAccountPoolVarName:               arc.ManagedDMAAccountPoolValue(),
		arc.MtpAccountVarName:                       ui.GaiaDMAPoolDefaultValue(),
		arc.ParentAccountVarName:                    arc.ParentDMAAccountValue(),
		arc.PlayAutoInstallAccountVarName:           arc.PlayAutoInstallDMAAccountValue(),
		arc.SharesheetPoolVarName:                   ui.GaiaDMAPoolDefaultValue(),
		assistant.AccountPoolVarName:                assistant.DmaAccountPoolValue(),
		calendar.GoogleCalendarAccountPoolVarName:   calendar.GoogleCalendarDMAAccountPoolValue(),
		calendar.UpcomingEventsAccountVarName:       calendar.UpcomingEventsDMAAccountValue(),
		connectivityfwdumps.GaiaLoginAccountVarName: arc.ManagedDMAAccountPoolValue(),
		dev.AccountVarName:                          ui.GaiaDMAPoolDefaultValue(),
		drivefs.AccountPoolVarName:                  ui.GaiaDMAPoolDefaultValue(),
		enterpriseconnectors.AshAccount1VarName:     enterpriseconnectors.AshDMAAccount1Value(),
		enterpriseconnectors.AshAccount2VarName:     enterpriseconnectors.AshDMAAccount2Value(),
		enterpriseconnectors.AshAccount3VarName:     enterpriseconnectors.AshDMAAccount3Value(),
		family.HohAccountVarName:                    family.HohDMAAccountValue(),
		family.ParentAccountVarName:                 family.ParentDMAAccountValue(),
		family.UnicornAllowlistAccountVarName:       family.UnicornAllowlistDMAAccountValue(),
		family.UnicornAccountVarName:                family.UnicornDMAAccountValue(),
		family.GellerAccountVarName:                 family.GellerDMAAccountValue(),
		family.GriffinAccountVarName:                family.GriffinDMAAccountValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullDMAAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullDMAAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnDMAAccountPoolValue(),
		floatingworkspace.AccountVarName:            ui.GaiaDMAPoolDefaultValue(),
		policy.ManagedUserAccountPoolVarName:        arc.ManagedDMAAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaDMAPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.GaiaDMAPoolDefaultValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosDMAAccountPoolValue(),
	}

	var regularPools = map[string]string{
		accountmanager.AccountPoolVarName:           accountmanager.AccountPoolValue(),
		arcappcompat.AccountVarName:                 arcappcompat.AccountValue(),
		arc.ChildAccountVarName:                     arc.ChildAccountValue(),
		arc.DrivefsPoolVarName:                      arc.DrivefsPoolValue(),
		arc.Managed3pEmmAccountVarName:              arc.Managed3pEmmAccountValue(),
		arc.ManagedAccountPoolVarName:               arc.ManagedAccountPoolValue(),
		arc.MtpAccountVarName:                       arc.MtpAccountValue(),
		arc.ParentAccountVarName:                    arc.ParentAccountValue(),
		arc.PlayAutoInstallAccountVarName:           arc.PlayAutoInstallAccountValue(),
		arc.SharesheetPoolVarName:                   arc.SharesheetPoolValue(),
		assistant.AccountPoolVarName:                assistant.AccountPoolValue(),
		calendar.GoogleCalendarAccountPoolVarName:   calendar.GoogleCalendarAccountPoolValue(),
		calendar.UpcomingEventsAccountVarName:       calendar.UpcomingEventsAccountValue(),
		connectivityfwdumps.GaiaLoginAccountVarName: connectivityfwdumps.GaiaLoginAccountValue(),
		dev.AccountVarName:                          dev.AccountValue(),
		drivefs.AccountPoolVarName:                  drivefs.AccountPoolValue(),
		enterpriseconnectors.AshAccount1VarName:     enterpriseconnectors.AshAccount1Value(),
		enterpriseconnectors.AshAccount2VarName:     enterpriseconnectors.AshAccount2Value(),
		enterpriseconnectors.AshAccount3VarName:     enterpriseconnectors.AshAccount3Value(),
		family.HohAccountVarName:                    family.HohAccountValue(),
		family.ParentAccountVarName:                 family.ParentAccountValue(),
		family.UnicornAllowlistAccountVarName:       family.UnicornAllowlistAccountValue(),
		family.UnicornAccountVarName:                family.UnicornAccountValue(),
		family.GellerAccountVarName:                 family.GellerAccountValue(),
		family.GriffinAccountVarName:                family.GriffinAccountValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnAccountPoolValue(),
		floatingworkspace.AccountVarName:            floatingworkspace.AccountValue(),
		policy.ManagedUserAccountPoolVarName:        policy.ManagedUserAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.CUJAccountPoolValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosAccountPoolValue(),
	}

	return dmaPools, regularPools
}

func enabled() bool {
	return strings.ToLower(dmaEnableVar.Value()) == "true"
}

// CredsFromPool returns proper credentials based on DMA status.
func CredsFromPool(pool string) string {
	dmaPools, regularPools := pools()

	dmaEnabled := enabled()
	if dmaEnabled {
		creds, ok := dmaPools[pool]
		if !ok {
			panic(fmt.Sprintf("Pool %q not onboard DMA yet", pool))
		}

		return creds
	}
	creds, ok := regularPools[pool]
	if !ok {
		panic(fmt.Sprintf("Pool %q not convert to global runtime variable yet", pool))
	}

	return creds
}

// UserPassFromPool returns a random username, password, error (if present) from pool.
func UserPassFromPool(pool string) (user, pass string, err error) {
	creds := CredsFromPool(pool)
	cred, err := credconfig.PickRandomCreds(creds)
	if err != nil {
		return "", "", err
	}

	return cred.User, cred.Pass, nil
}
