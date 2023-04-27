// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filecheck

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/moblab"
	"chromiumos/tast/testing"
)

// CheckStatefulFiles verifies file permissions on the stateful partition.
// When it returns an error, it's likely because some other test broke the stateful partition isolation.
func CheckStatefulFiles(ctx context.Context, outDir string) []error {
	const (
		root      = "/mnt/stateful_partition"
		errorFile = "stateful_errors.txt"
		maxErrors = 5 // max to print
	)

	// The basic approach here is to specify patterns for paths within a top-level directory, and then add a catch-all
	// Tree pattern that checks anything in the directory that wasn't already explicitly checked or skipped.
	// Any top-level directories not explicitly handled are matched by the final AllPaths pattern.
	patterns := []*Pattern{
		NewPattern(Path("dev_image"), SkipChildren()),     // only exists for dev images
		NewPattern(Path("dev_image_old"), SkipChildren()), // only exists for dev images
		NewPattern(Path("dev_image_new"), SkipChildren()), // only exists for dev images

		NewPattern(Path("encrypted/chronos"), Users("chronos"), Groups("chronos"), Mode(0755), SkipChildren()), // contents checked by security.UserFiles*

		NewPattern(Path("encrypted/var/cache/app_pack"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		// TODO(crbug.com/905719): Check for a specific user:group and mode.
		NewPattern(Path("encrypted/var/cache/camera"), Users("chronos", "root"), NotMode(02), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/device_local_account_component_policy"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/device_local_account_extensions"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/device_local_account_external_policy_data"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/device_policy_external_data"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/display_profiles"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/edb"), Users("root"), Groups("portage"), Mode(0755), SkipChildren()),
		NewPattern(Tree("encrypted/var/cache/echo"), Users("root"), NotMode(022)),
		// Temporary directory to create external_cache. Most of time, it's owned by root, but switched to chronos just before its renaming.
		// See also crx-import.sh for details.
		NewPattern(Path("encrypted/var/cache/external_cache.tmp"), Users("chronos", "root"), Groups("chronos", "root"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/external_cache"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Tree("encrypted/var/cache/hermes"), Users("modem"), Groups("modem"), NotMode(022)),
		NewPattern(Tree("encrypted/var/cache/ldconfig"), Users("root"), Groups("root"), NotMode(077)),
		NewPattern(Tree("encrypted/var/cache/modemfwd"), Users("modem"), Groups("modem"), NotMode(022)),
		NewPattern(Tree("encrypted/var/cache/modem-utilities"), Users("shill-scripts"), Groups("shill-scripts"), Mode(0664)),
		NewPattern(Path("encrypted/var/cache/shared_extensions"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Tree("encrypted/var/cache/shill"), Users("shill"), Groups("shill"), NotMode(022)),
		NewPattern(Path("encrypted/var/cache/signin_profile_component_policy"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Path("encrypted/var/cache/signin_profile_extensions"), Users("chronos"), Groups("chronos"), Mode(0700), SkipChildren()),
		NewPattern(Tree("encrypted/var/cache"), Users("root"), Groups("root"), NotMode(022)),

		NewPattern(Tree("encrypted/var/coredumps"), Users("chronos"), Groups("chronos"), NotMode(077)),

		// TODO(b/205582301) - Some Bluetooth config files are initialized with group write permission. Temporarily don't check the group settings.
		NewPattern(Tree("encrypted/var/lib/bluetooth"), Users("bluetooth"), NotMode(007)),
		NewPattern(Tree("encrypted/var/lib/bootlockbox"), Users("bootlockboxd"), Groups("bootlockboxd"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/chaps"), Users("chaps"), Groups("chronos-access"), NotMode(022)),
		NewPattern(Path("encrypted/var/lib/cras"), Users("cras"), Groups("cras"), Mode(0755)),                 // directory itself
		NewPattern(Tree("encrypted/var/lib/cras"), Users("cras"), Groups("cras"), Mode(0644), SkipChildren()), // children
		NewPattern(Tree("encrypted/var/lib/chaps/database"), Users("chaps"), Groups("chronos-access"), NotMode(027)),
		NewPattern(Path("encrypted/var/lib/dhcpcd"), Users("dhcp"), Groups("dhcp"), Mode(0775)),
		NewPattern(Tree("encrypted/var/lib/dhcpcd"), Users("dhcp"), Groups("dhcp"), NotMode(0113)),
		NewPattern(Path("encrypted/var/lib/gentoo"), Users("root"), NotMode(022), SkipChildren()),
		NewPattern(Tree("encrypted/var/lib/hpsd"), Users("hpsd"), Groups("hpsd"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/imageloader"), Users("imageloaderd"), Groups("imageloaderd"), NotMode(022)),
		// TODO(chromium:1197973): Re-add permissions checks for /var/lib/metrics
		NewPattern(Tree("encrypted/var/lib/metrics"), SkipChildren()),
		NewPattern(Tree("encrypted/var/lib/ml_core"), Users("ml-core"), Groups("ml-core"), NotMode(02)),
		NewPattern(Tree("encrypted/var/lib/ml_core/opencl_cache"), Users("ml-core"), Groups("ml-core"), NotMode(02)),
		NewPattern(Tree("encrypted/var/lib/ml_service"), Users("ml-service"), Groups("ml-service"), NotMode(02)),
		NewPattern(Tree("encrypted/var/lib/modemfwd"), Users("modem"), Groups("modem"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/oobe_config_restore"), Users("oobe_config_restore"), Groups("oobe_config_restore"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/oobe_config_save"), Users("oobe_config_save"), Groups("oobe_config_save"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/power_manager"), Users("power"), Groups("power"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/private_computing"), Users("private_computing"), Groups("private_computing"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/shill"), Users("shill"), Groups("shill"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/timezone"), Users("chronos", "root"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/tpm"), Users("root"), Groups("root"), NotMode(077)),
		NewPattern(Tree("encrypted/var/lib/vm_cicerone"), Users("vm_cicerone"), Groups("vm_cicerone"), NotMode(022)),
		NewPattern(Tree("encrypted/var/lib/vtpm"), Users("vtpm"), Groups("vtpm"), NotMode(022)),
		NewPattern(Path("encrypted/var/lib/whitelist"), Users("root"), Groups("policy-readers"), Mode(0750)),      // directory itself
		NewPattern(Tree("encrypted/var/lib/whitelist"), Users("root"), Groups("root"), NotMode(022)),              // children
		NewPattern(Path("encrypted/var/lib/devicesettings"), Users("root"), Groups("policy-readers"), Mode(0750)), // directory itself
		NewPattern(Tree("encrypted/var/lib/devicesettings"), Users("root"), Groups("root"), NotMode(022)),         // children
		NewPattern(Tree("encrypted/var/lib"), Users("root"), Groups("root"), NotMode(022)),

		NewPattern(Tree("encrypted/var/log/asan"), Users("root"), Groups("root"), Mode(0777|os.ModeSticky)),
		NewPattern(Tree("encrypted/var/log/chrome/Crash Reports/uploads.log"), Users("root"), Groups("root"), Mode(0644)),
		// Allow group read so the Chrome browser can send feedback reports.
		NewPattern(Tree("encrypted/var/log/chrome/Crash Reports"), Users("chronos"), Groups("chronos"), NotMode(037)),
		NewPattern(Tree("encrypted/var/log/chrome"), Users("chronos"), Groups("chronos"), NotMode(022)),
		NewPattern(Path("encrypted/var/log/emerge.log"), Users("portage"), Groups("portage"), Mode(0660)),
		NewPattern(Tree("encrypted/var/log/lacros/Crash Reports/uploads.log"), Users("root"), Groups("root"), Mode(0644)),
		NewPattern(Tree("encrypted/var/log/lacros/Crash Reports"), Users("chronos"), Groups("chronos"), NotMode(077)),
		NewPattern(Tree("encrypted/var/log/lacros"), Users("chronos"), Groups("chronos"), NotMode(022)),
		NewPattern(Tree("encrypted/var/log/lorgnette"), Users("saned"), Groups("scanner"), NotMode(022)),
		NewPattern(Tree("encrypted/var/log/metrics"), Users("root", "chronos", "metrics", "shill"), NotMode(022)),
		NewPattern(Tree("encrypted/var/log/modemfwd"), Users("modem"), Groups("modem"), NotMode(022)),
		NewPattern(Tree("encrypted/var/log/power_manager"), Users("power"), Groups("power"), NotMode(022)),
		NewPattern(Tree("encrypted/var/log/tcsd"), Users("tss"), Groups("tss"), NotMode(022)),
		NewPattern(Path("encrypted/var/log/usbmon"), Users("root", "tcpdump"), Groups("root", "tcpdump"), SkipChildren()), // only created by tests
		NewPattern(Path("encrypted/var/log/vmlog"), Users("metrics"), Groups("metrics"), Mode(0755)),                      // directory itself
		NewPattern(Tree("encrypted/var/log/vmlog"), Users("metrics"), Groups("metrics"), Mode(0644)),                      // children
		NewPattern(Path("encrypted/var/log"), Users("root"), Groups("syslog"), Mode(0775|os.ModeSticky)),                  // directory itself
		NewPattern(Tree("encrypted/var/log"), Users("syslog", "root"), Groups("syslog", "root"), NotMode(022)),            // children

		NewPattern(Path("encrypted/var/spool/crash"), Users("root"), Groups("crash-access"), Mode(0770|os.ModeSetgid)), // directory itself
		NewPattern(Tree("encrypted/var/spool/crash"), Users("root"), Groups("crash-access"), NotMode(002)),             // children

		NewPattern(Path("encrypted/var/spool/support"), Users("chronos"), Groups("root"), NotMode(022)),  // directory
		NewPattern(Tree("encrypted/var/spool/support"), Users("chronos"), Groups("chronos"), Mode(0644)), // children

		NewPattern(Path("encrypted/var/tmp"), Users("root"), Groups("root"), Mode(0777|os.ModeSticky), SkipChildren()),

		NewPattern(Tree("encrypted"), Users("root"), NotMode(022)),
		NewPattern(PathRegexp(`^encrypted\.`), Users("root"), Groups("root"), Mode(0600)),

		NewPattern(Tree("etc"), Users("root"), NotMode(022)),

		NewPattern(Path("home/.shadow"), Users("root"), Groups("root"), Mode(0700), SkipChildren()),
		NewPattern(Path("home/root"), Users("root"), Groups("root"), Mode(0751|os.ModeSticky)),                // directory itself
		NewPattern(Tree("home/root"), Users("root"), Groups("root"), Mode(0700), SkipChildren()),              // top-level children
		NewPattern(Path("home/user"), Users("root"), Groups("root"), Mode(0755)),                              // directory itself
		NewPattern(Tree("home/user"), Users("chronos"), Groups("chronos-access"), Mode(0750), SkipChildren()), // top-level children
		NewPattern(Tree("home"), Users("root"), Groups("root"), NotMode(022)),

		NewPattern(Path("unencrypted/apkcache"), Mode(0700), SkipChildren()),
		NewPattern(Tree("unencrypted/attestation"), Users("attestation", "root"), NotMode(022)),
		NewPattern(Path("unencrypted/preserve"), Users("root"), NotMode(02)),                              // directory itself
		NewPattern(Path("unencrypted/preserve/cros-update"), SkipChildren()),                              // only exists for testing
		NewPattern(Path("unencrypted/preserve/log"), SkipChildren()),                                      // only exists for testing
		NewPattern(Path("unencrypted/preserve/rollback_data"), Users("oobe_config_save"), SkipChildren()), // only exists after rollback
		NewPattern(Tree("unencrypted/preserve"), Users("attestation", "root"), NotMode(022)),              // other children
		NewPattern(Path("unencrypted/userspace_swap.tmp"), Users("root"), SkipChildren()),
		NewPattern(Tree("unencrypted"), Users("root"), NotMode(022)),

		NewPattern(Path("var_overlay"), SkipChildren()), // only exists for dev images

		// This file can be created by
		// https://source.corp.google.com/chromeos_public/src/platform/factory/py/gooftool/wipe.py.
		// TODO(crbug.com/1083285): Avoid creating this file with 0666 permissions.
		NewPattern(Path("wipe_mark_file"), Mode(0666)),

		NewPattern(Root(), Users("root"), Groups("root"), Mode(0755)), // stateful_partition directory itself
		NewPattern(AllPaths(), Users("root"), NotMode(022)),           // everything else not already matched
	}

	// prependPatterns prepends the supplied patterns to the main patterns slice.
	prependPatterns := func(newPatterns ...*Pattern) { patterns = append(newPatterns, patterns...) }

	if _, err := user.Lookup("tss"); err == nil {
		prependPatterns(NewPattern(Tree("var-overlay/lib/tpm"), Users("tss"), NotMode(022)))
	}

	if _, err := user.Lookup("tpm_manager"); err == nil {
		prependPatterns(
			NewPattern(Path("encrypted/var/lib/tpm_manager"), Users("tpm_manager"), Groups("tpm_manager"), NotMode(022)),
			NewPattern(Path("encrypted/var/lib/tpm_manager/.allowed"), Users("tpm_manager"), Groups("tpm_manager"), NotMode(077)),
			NewPattern(Path("encrypted/var/lib/tpm_manager/local_tpm_data"), Users("tpm_manager"), Groups("tpm_manager"), NotMode(077)),
			NewPattern(Path("encrypted/var/lib/tpm_manager/local_tpm_data.tast-hwsec-backup"), Users("tpm_manager"), Groups("tpm_manager"), NotMode(077)),
			NewPattern(Tree("unencrypted/tpm_manager"), Users("tpm_manager"), Groups("tpm_manager"), NotMode(022)))
	}

	if _, err := user.Lookup("tpm2-simulator"); err == nil {
		prependPatterns(
			NewPattern(Path("unencrypted/tpm2-simulator"), Users("tpm2-simulator"), Groups("tpm2-simulator"), NotMode(022)),
			NewPattern(Path("unencrypted/tpm2-simulator/NVChip"), Users("tpm2-simulator"), Groups("tpm2-simulator"), NotMode(022)),
			NewPattern(Path("unencrypted/tpm2-simulator/NVChip_mount"), Users("tpm2-simulator"), Groups("tpm2-simulator"), NotMode(022)))
	}

	if _, err := user.Lookup("trunks"); err == nil {
		prependPatterns(
			NewPattern(Path("encrypted/var/lib/trunks"), Users("trunks"), Groups("trunks"), NotMode(022)))
	}

	if _, err := user.Lookup("biod"); err == nil {
		prependPatterns(
			NewPattern(Tree("encrypted/var/log/bio_crypto_init"), Users("biod", "root"), Groups("biod", "root"), NotMode(022)),
			NewPattern(Tree("encrypted/var/log/biod"), Users("biod", "root"), Groups("biod", "root"), NotMode(022)))
	}

	if _, err := user.Lookup("buffet"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/lib/buffet"), Users("buffet"), Groups("buffet"), NotMode(02)))
	}

	if _, err := user.Lookup("cups"); err == nil {
		prependPatterns(
			NewPattern(Tree("encrypted/var/cache/cups"), Users("cups"), Groups("cups", "nobody"), NotMode(02)),
			NewPattern(Tree("encrypted/var/spool/cups"), Users("cups"), Groups("cups", "nobody"), NotMode(02)))
	}

	if _, err := user.Lookup("android-root"); err == nil {
		prependPatterns(
			NewPattern(Tree("unencrypted/art-data"), Users("android-root", "root"), NotMode(022)))
	}

	if _, err := user.Lookup("cdm-oemcrypto"); err == nil {
		prependPatterns(NewPattern(Path("encrypted/var/lib/oemcrypto"), Users("cdm-oemcrypto"), Groups("cdm-oemcrypto"), Mode(0700), SkipChildren()))
	}

	if _, err := user.Lookup("fwupd"); err == nil {
		prependPatterns(
			NewPattern(Path("encrypted/var/cache/fwupd"), Users("fwupd"), Groups("fwupd"), SkipChildren()),
			NewPattern(Path("encrypted/var/lib/fwupd"), Users("fwupd"), Groups("fwupd"), SkipChildren()))
	}

	if _, err := user.Lookup("dlcservice"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/cache/dlc"), Users("dlcservice"), Groups("dlcservice"), NotMode(022)))
		// encrypted dlc-images is created by dev_utils.sh script prior to bind
		// mounting unencrypted dlc-images directory.
		prependPatterns(NewPattern(Tree("encrypted/var/cache/dlc-images"), Users("dlcservice"), Groups("dlcservice"), NotMode(022)))
		prependPatterns(NewPattern(Tree("encrypted/var/lib/dlcservice"), Users("dlcservice"), Groups("dlcservice"), NotMode(022)))
		prependPatterns(NewPattern(Tree("unencrypted/dlc-factory-images"), Users("dlcservice"), Groups("dlcservice"), NotMode(022)))
	}

	if _, err := user.Lookup("wilco_dtc"); err == nil {
		prependPatterns(NewPattern(Path("encrypted/var/lib/wilco/storage.img"), Users("wilco_dtc"), Groups("wilco_dtc"), NotMode(022)))
	}

	if _, err := user.Lookup("cros_healthd"); err == nil {
		prependPatterns(NewPattern(Path("encrypted/var/cache/diagnostics"), Users("cros_healthd"), Groups("cros_healthd"), SkipChildren()))
	}

	if _, err := user.Lookup("missived"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/cache/reporting"), Users("missived"), Groups("missived"), NotMode(022)))
		prependPatterns(NewPattern(Tree("encrypted/var/spool/reporting"), Users("missived"), Groups("missived"), NotMode(022)))
	}

	if _, err := user.Lookup("displaylink"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/log/displaylink"), Users("displaylink"), Groups("displaylink"), NotMode(022), SkipChildren()))
	}

	if _, err := user.Lookup("sound_card_init"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/lib/sound_card_init"), Users("sound_card_init"), Groups("sound_card_init"), NotMode(022)))
	}

	if _, err := user.Lookup("rmtfs"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/lib/rmtfs"), Users("rmtfs"), NotMode(022)))
	}

	if _, err := user.Lookup("rmad"); err == nil {
		prependPatterns(NewPattern(Tree("encrypted/var/lib/rmad"), Users("rmad"), Groups("rmad"), NotMode(022)))
		prependPatterns(NewPattern(Tree("unencrypted/rma-data"), Users("rmad"), Groups("rmad"), NotMode(022)))
	}

	if moblab.IsMoblab() {
		// On moblab devices, there are additional user dirs and tons of stuff (MySQL, etc.) in /var.
		prependPatterns(
			NewPattern(Tree("home/chronos"), Users("chronos", "root")),
			NewPattern(Tree("home/moblab"), Users("moblab", "root")),
			NewPattern(Tree("var"), SkipChildren()))
	}

	testing.ContextLog(ctx, "Checking ", root)
	problems, numPaths, err := Check(ctx, root, patterns)
	testing.ContextLogf(ctx, "Scanned %d path(s)", numPaths)
	if err != nil {
		return []error{errors.Wrapf(err, "failed to check %v", root)}
	}

	f, err := os.Create(filepath.Join(outDir, errorFile))
	if err != nil {
		return []error{errors.Wrap(err, "failed to create error file")}
	}
	defer f.Close()
	for path, msgs := range problems {
		if _, err := fmt.Fprintf(f, "%v: %v\n", path, strings.Join(msgs, ", ")); err != nil {
			return []error{errors.Wrap(err, "failed to write error file")}
		}
	}

	var result []error
	for path, msgs := range problems {
		if len(result) > maxErrors {
			testing.ContextLogf(ctx, "Too many errors; aborting (see %v)", errorFile)
			break
		}
		result = append(result, errors.Errorf("%v: %v", path, strings.Join(msgs, ", ")))
	}
	return result
}
