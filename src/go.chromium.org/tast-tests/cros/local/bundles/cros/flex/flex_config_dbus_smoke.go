// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flex

import (
	"context"
	"os"
	"time"

	oobeconfigpb "go.chromium.org/chromiumos/system_api/oobe_config_proto"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

const (
	dbusName      = "org.chromium.OobeConfigRestore"
	dbusPath      = "/org/chromium/OobeConfigRestore"
	dbusInterface = "org.chromium.OobeConfigRestore"

	flexConfigDirPath  = "/mnt/stateful_partition/unencrypted/flex_config"
	flexConfigFilePath = "/mnt/stateful_partition/unencrypted/flex_config/config.json"
	flexConfigJSONData = "{ \"enrollmentToken\" : \"test-enrollment-token\" }"

	oobeConfigRestoreUID = 20121
	oobeConfigRestoreGID = 20121
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FlexConfigDbusSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify basic functionality of OobeConfigRestore Flex Config DBus methods",
		Contacts: []string{
			"cros-onboarding-team@google.com",
			"jacksontadie@google.com",
		},
		BugComponent: "b:1271043", // Chrome OS Server Projects > Enterprise Management >> Chrome Commercial Backend >> Onboarding >> Enterprise Enrollment
		Fixture:      fixture.CleanOwnership,
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"reven_oobe_config"},
	})
}

func FlexConfigDbusSmoke(ctx context.Context, s *testing.State) {
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Verify oobe_config_restore job is running.
	if err := upstart.CheckJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failure when checking that oobe_config_restore is running: ", err)
	}

	defer cleanUpFlexConfigDir(cleanUpCtx, s)

	s.Log("Seeding flex config JSON")
	if err := os.Mkdir(flexConfigDirPath, 0777); err != nil {
		s.Fatalf("Failed to create %s directory: %v", flexConfigDirPath, err)
	}
	if err := os.WriteFile(flexConfigFilePath, []byte(flexConfigJSONData), 0777); err != nil {
		s.Fatalf("Failed to create %s: %v", flexConfigFilePath, err)
	}
	if err := os.Chown(flexConfigFilePath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		s.Fatalf("Failed to chown %s: %v", flexConfigFilePath, err)
	}
	if err := os.Chown(flexConfigDirPath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		s.Fatalf("Failed to chown %s: %v", flexConfigDirPath, err)
	}

	// Have to restart job after creating flex_config files as upstart config
	// conditionally bind-mounts the flex_config dir only if it's present.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore daemon: ", err)
	}

	_, obj, err := dbusutil.Connect(ctx, dbusName, dbus.ObjectPath(dbusPath))
	if err != nil {
		s.Fatalf("Failed to connect to %s: %v", dbusName, err)
	}

	s.Log("Calling ProcessAndGetOobeAutoConfig DBus method")
	config, err := readFlexConfig(ctx, obj)
	if err != nil {
		s.Fatal("Error reading Flex config: ", err)
	}
	if config != flexConfigJSONData {
		s.Fatalf("Data from ProcessAndGetOobeAutoConfig does not match. Actual: %s - Expected: %s", config, flexConfigJSONData)
	}

	s.Log("Calling DeleteFlexOobeConfig DBus method")
	if err = deleteFlexConfig(ctx, obj); err != nil {
		s.Fatal("Failed to call DeleteFlexOobeConfig method: ", err)
	}

	// Attempt to get Flex config again through DBus to assert it's empty.
	config, err = readFlexConfig(ctx, obj)
	if err != nil {
		s.Fatal("Error reading Flex config: ", err)
	}
	if config != "" {
		s.Fatal("Flex config is not empty after attempted deletion: ", config)
	}

	// Verify directly via file system that the config file is deleted.
	// TODO(b/324093179): Update this check to new location once token is moved
	// to encrypted stateful.
	_, err = os.Stat(flexConfigFilePath)
	if os.IsExist(err) {
		s.Fatal("Flex config still exists after calling DeleteFlexConfig")
	}
	if err != nil && !os.IsNotExist(err) {
		s.Fatal("Unexpected error when trying to stat Flex config file: ", err)
	}

	s.Log("Calling DeleteFlexConfig DBus method again to test for FileNotFound response error")
	err = deleteFlexConfig(ctx, obj)
	if err == nil || err.Error() != "Flex OOBE config not found." {
		s.Fatal("DeleteFlexOobeConfig error was not \"config not found\": ", err)
	}
}

func readFlexConfig(ctx context.Context, obj dbus.BusObject) (string, error) {
	var responseError int32
	var buf []byte
	if err := obj.CallWithContext(ctx, dbusInterface+".ProcessAndGetOobeAutoConfig", 0).Store(&responseError, &buf); err != nil {
		return "", errors.Wrap(err, "failed to call the DBus ProcessAndGetOobeAutoConfig method")
	}
	if responseError != 0 {
		return "", errors.Errorf("Non-zero ProcessAndGetOobeAutoConfig error: %d", responseError)
	}
	getResponse := &oobeconfigpb.OobeRestoreData{}
	if err := proto.Unmarshal(buf, getResponse); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal OobeRestoreData")
	}
	return getResponse.ChromeConfigJson, nil
}

func deleteFlexConfig(ctx context.Context, obj dbus.BusObject) error {
	err := obj.CallWithContext(ctx, dbusInterface+".DeleteFlexOobeConfig", 0).Store()
	return err
}

// cleanUpFlexConfigDir ensures the flex config files are deleted during cleanup.
//
// TODO(b/324093179): Update this to encrypted stateful partition path, once
// config is moved there.
func cleanUpFlexConfigDir(ctx context.Context, s *testing.State) {
	s.Log("Cleaning up Flex config dir")
	if err := os.RemoveAll(flexConfigDirPath); err != nil {
		s.Fatal("Failed to delete Flex config dir: ", err)
	}
}
