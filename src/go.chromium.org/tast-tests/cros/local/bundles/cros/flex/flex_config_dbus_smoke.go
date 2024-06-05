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

	flexConfigInitialDirPath  = "/mnt/stateful_partition/unencrypted/flex_config"
	flexConfigInitialFilePath = "/mnt/stateful_partition/unencrypted/flex_config/config.json"
	// Path that flex_config will be in after moved to encrypted stateful partition (ESP)
	// on oobe_config_restore startup.
	flexConfigESPDirPath  = "/var/lib/oobe_config_restore/flex_config"
	flexConfigESPFilePath = "/var/lib/oobe_config_restore/flex_config/config.json"

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

	defer func() {
		s.Log("Cleaning up Flex config dirs")
		if err := cleanUpFlexConfigDirs(cleanUpCtx); err != nil {
			s.Error("Failed to clean up Flex config dirs: ", err)
		}
	}()

	s.Log("Seeding flex config JSON")
	if err := os.Mkdir(flexConfigInitialDirPath, 0740); err != nil {
		s.Fatalf("Failed to create %s directory: %v", flexConfigInitialDirPath, err)
	}
	if err := os.WriteFile(flexConfigInitialFilePath, []byte(flexConfigJSONData), 0640); err != nil {
		s.Fatalf("Failed to create %s: %v", flexConfigInitialFilePath, err)
	}
	if err := os.Chown(flexConfigInitialFilePath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		s.Fatalf("Failed to chown %s: %v", flexConfigInitialFilePath, err)
	}
	if err := os.Chown(flexConfigInitialDirPath, oobeConfigRestoreUID, oobeConfigRestoreGID); err != nil {
		s.Fatalf("Failed to chown %s: %v", flexConfigInitialDirPath, err)
	}

	// Have to restart job after creating flex_config files as upstart config
	// conditionally bind-mounts the flex_config dir only if it's present.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore daemon: ", err)
	}

	// Verify flex_config has been migrated to encrypted stateful partition after
	// oobe_config_restore restart.
	exists, err := pathExists(flexConfigInitialFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in unencrypted stateful partition: ", err)
	}
	if exists {
		s.Fatal("Flex config has not been deleted from unencrypted stateful partition")
	}
	exists, err = pathExists(flexConfigESPFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in encrypted stateful partition: ", err)
	}
	if !exists {
		s.Fatal("Flex config has not been moved to encrypted stateful partition")
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
	exists, err = pathExists(flexConfigESPFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in encrypted stateful partition: ", err)
	}
	if exists {
		s.Fatal("Flex config still exists after calling DeleteFlexConfig")
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

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return false, errors.Wrapf(err, "unexpected error when trying to stat %s", path)
	}
	return err == nil, nil
}

// cleanUpFlexConfigDirs ensures the flex config files are deleted during cleanup.
func cleanUpFlexConfigDirs(ctx context.Context) error {
	if err := os.RemoveAll(flexConfigInitialDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete Flex config dir %s", flexConfigInitialDirPath)
	}
	if err := os.RemoveAll(flexConfigESPDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete Flex config dir %s", flexConfigESPDirPath)
	}
	return nil
}
