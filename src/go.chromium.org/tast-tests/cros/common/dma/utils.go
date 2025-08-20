// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dma

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast-tests/cros/common/accountmanager"
	"go.chromium.org/tast-tests/cros/common/ambient"
	"go.chromium.org/tast-tests/cros/common/arc"
	"go.chromium.org/tast-tests/cros/common/arcappcompat"
	"go.chromium.org/tast-tests/cros/common/assistant"
	"go.chromium.org/tast-tests/cros/common/calendar"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/connectivityfwdumps"
	"go.chromium.org/tast-tests/cros/common/crossdevice"
	"go.chromium.org/tast-tests/cros/common/dev"
	"go.chromium.org/tast-tests/cros/common/drivefs"
	"go.chromium.org/tast-tests/cros/common/enterpriseconnectors"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/common/filemanager"
	"go.chromium.org/tast-tests/cros/common/floatingworkspace"
	"go.chromium.org/tast-tests/cros/common/glanceables"
	"go.chromium.org/tast-tests/cros/common/nearbyshare"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
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
		ambient.AccountVarName:                      assistant.DmaAccountPoolValue(),
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
		crossdevice.DefaultCrossDevicePoolVarName:   crossdevice.DmaDefaultCrossDevicePoolValue(),
		crossdevice.SmartLockPoolVarName:            crossdevice.DmaSmartLockPoolValue(),
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
		family.EduAccountVarName:                    family.EduDMAAccountValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullDMAAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullDMAAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnDMAAccountPoolValue(),
		floatingworkspace.AccountVarName:            ui.GaiaDMAPoolDefaultValue(),
		glanceables.RegularAccountVarName:           glanceables.RegularDMAAccountValue(),
		glanceables.StudentAccountVarName:           glanceables.StudentDMAAccountValue(),
		glanceables.TeacherAccountVarName:           glanceables.TeacherDMAAccountValue(),
		nearbyshare.CrosAccountPoolVarName:          nearbyshare.DmaCrosAccountPoolValue(),
		nearbyshare.CrosAccount2PoolVarName:         nearbyshare.DmaCrosAccount2PoolValue(),
		nearbyshare.AndroidAccountPoolVarName:       nearbyshare.DmaAndroidAccountPoolValue(),
		nearbyshare.DevAndroidAccountPoolVarName:    nearbyshare.DmaDevAndroidAccountPoolValue(),
		nearbyshare.ProdAndroidAccountPoolVarName:   nearbyshare.DmaProdAndroidAccountPoolValue(),
		policy.ManagedUserAccountPoolVarName:        arc.ManagedDMAAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaDMAPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.GaiaDMAPoolDefaultValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosDMAAccountPoolValue(),
		arc.ManagedDMSAccountPoolVarName:            arc.ManagedDMSAccountPoolValue(),
	}

	var regularPools = map[string]string{
		accountmanager.AccountPoolVarName:           accountmanager.AccountPoolValue(),
		ambient.AccountVarName:                      ambient.AccountValue(),
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
		crossdevice.DefaultCrossDevicePoolVarName:   crossdevice.DefaultCrossDevicePoolValue(),
		crossdevice.SmartLockPoolVarName:            crossdevice.SmartLockPoolValue(),
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
		family.EduAccountVarName:                    family.EduAccountValue(),
		filemanager.FullAccountPoolVarName:          filemanager.FullAccountPoolValue(),
		filemanager.OrgFullAccountPoolVarName:       filemanager.OrgFullAccountPoolValue(),
		filemanager.WarnAccountPoolVarName:          filemanager.WarnAccountPoolValue(),
		floatingworkspace.AccountVarName:            floatingworkspace.AccountValue(),
		glanceables.RegularAccountVarName:           glanceables.RegularAccountValue(),
		glanceables.StudentAccountVarName:           glanceables.StudentAccountValue(),
		glanceables.TeacherAccountVarName:           glanceables.TeacherAccountValue(),
		nearbyshare.CrosAccountPoolVarName:          nearbyshare.CrosAccountPoolValue(),
		nearbyshare.CrosAccount2PoolVarName:         nearbyshare.CrosAccount2PoolValue(),
		nearbyshare.AndroidAccountPoolVarName:       nearbyshare.AndroidAccountPoolValue(),
		nearbyshare.DevAndroidAccountPoolVarName:    nearbyshare.DevAndroidAccountPoolValue(),
		nearbyshare.ProdAndroidAccountPoolVarName:   nearbyshare.ProdAndroidAccountPoolValue(),
		policy.ManagedUserAccountPoolVarName:        policy.ManagedUserAccountPoolValue(),
		ui.GaiaPoolDefaultVarName:                   ui.GaiaPoolDefaultValue(),
		ui.CUJAccountPoolVarName:                    ui.CUJAccountPoolValue(),
		wallpaper.GooglePhotosAccountPoolVarName:    wallpaper.GooglePhotosAccountPoolValue(),
		arc.ManagedDMSAccountPoolVarName:            arc.ManagedDMSAccountPoolValue(),
	}

	return dmaPools, regularPools
}

// tapePools returns pool->pool mapping from  non-dma account pools to dma account pools.
func tapePools() map[string]string {
	return map[string]string{
		tape.DefaultManaged:                    tape.DmaDefaultManaged,
		tape.BuiltInCertProvisioningTesting:    tape.DmaBuiltInCertProvisioningTesting,
		tape.BuiltInCertProvisioningAPITesting: tape.DmaBuiltInCertProvisioningAPITesting,
		tape.DeviceTrustDisabled:               tape.DmaDeviceTrustDisabled,
		tape.DeviceTrustEnabled:                tape.DmaDeviceTrustEnabled,
		tape.ChromeosbytebotCom:                tape.DmaChromeosbytebotCom,
	}
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

// UserPassFromPoolWithRotation returns a username, password, error (if present) from pool based on rotation.
// If rotation fails, it will fallback to random selection.
func UserPassFromPoolWithRotation(ctx context.Context, pool string, rotationDays int) (user, pass string, err error) {
	creds := CredsFromPool(pool)

	cred, rotErr := credconfig.PickRotatingCreds(creds, rotationDays)
	if rotErr != nil {
		testing.ContextLogf(ctx, "Warning: Failed to pick rotating credential for pool %q: %v. Falling back to random selection", pool, rotErr)

		// Fallback to the standard random selection function
		return UserPassFromPool(pool)
	}

	return cred.User, cred.Pass, nil
}

// TapePool returns appropriate account pool based on DMA status.
// This function is similar to CredsFromPool, however it is intended for
// use with Tape API which has different semantics and separate account database.
func TapePool(pool string) string {
	if !enabled() {
		return pool
	}
	tapePools := tapePools()
	dmaPool, ok := tapePools[pool]
	if !ok {
		panic(fmt.Sprintf("Tape pool %q not onboarded DMA yet", pool))
	}
	return dmaPool
}
