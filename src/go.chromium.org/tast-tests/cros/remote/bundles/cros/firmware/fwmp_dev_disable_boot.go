// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwmpDevDisableBoot,
		Desc: "Verify that firmware management parameters (FWMP) can restrict developer mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Timeout:      30 * time.Minute,
		Fixture:      fixture.DevMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"tpm2"},
	})
}

func FwmpDevDisableBoot(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)
	h := v.Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	setFWMP := func(ctx context.Context, flags string) error {
		s.Log("Taking ownership")
		if err := s.DUT().Conn().CommandContext(ctx, "tpm_manager_client", "take_ownership").Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to take ownership")
		}

		/*
			Allow some delay to ensure that this command has fully propagated.
			Without this delay, the following error would likely appear when
			setting firmware management parameters:
			'Failed to call SetFirmwareManagementParameters: (dbus, org.freedesktop.DBus.Error.ServiceUnknown,
			Error calling D-Bus method: org.chromium.InstallAttributesInterface.SetFirmwareManagementParameters:
			The name org.chromium.UserDataAuth was not provided by any .service files)'
		*/
		s.Logf("Sleeping for %s", 5*time.Second)
		// GoBigSleepLint: This is a temporary sleep until a better solution can be found.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}

		// Set FWMP flags in a poll to increase the chances of success.
		s.Log("Setting firmware management parameters")
		reOwnerPassword := regexp.MustCompile(`flags:\s*(0|1)`)
		var currentFlagVal [][]byte
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := s.DUT().Conn().CommandContext(ctx, "cryptohome", "--action=set_firmware_management_parameters", "--flags=0x"+flags).Run(ssh.DumpLogOnError); err != nil {
				return errors.Wrapf(err, "failed to set firmware management parameters with flags 0x%s", flags)
			}

			// Verify the flags have been set as expected.
			out, err := s.DUT().Conn().CommandContext(ctx, "cryptohome", "--action=get_firmware_management_parameters").Output(ssh.DumpLogOnError)
			if err != nil {
				return errors.Wrap(err, "failed to get firmware management parameter flags")
			}

			currentFlagVal = reOwnerPassword.FindSubmatch(out)
			if currentFlagVal == nil {
				return errors.Errorf("no match found with regex %q in the following output: %s", reOwnerPassword, out)
			}
			if string(currentFlagVal[1]) != flags {
				return errors.Wrapf(err, "flags haven't been set correctly: expected flags to be %q but got the following output: %s", flags, string(out))
			}

			return nil
		}, &testing.PollOptions{Timeout: 25 * time.Second, Interval: 3 * time.Second}); err != nil {
			return err
		}

		return nil
	}

	// Verify that TPM owner password is present.
	out, err := s.DUT().Conn().CommandContext(ctx, "tpm_manager_client", "status", "--nonsensitive").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get TMP status: ", err)
	}
	reOwnerPassword := regexp.MustCompile(`is_owner_password_present: (\S*)`)
	ownerPasswordState := reOwnerPassword.FindSubmatch(out)
	if ownerPasswordState == nil {
		s.Fatalf("No match found with regex %q in the following output: %s", reOwnerPassword, out)
	}
	if string(ownerPasswordState[1]) != "true" {
		s.Fatal("TPM owner password is not present, and received output: ", string(out))
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create mode switcher: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	// Set DUT in "dev mode enable" state by setting TPM flags to "0x0" at the end of the test.
	defer func(cleanupCtx context.Context) {
		s.Log("Verifying DUT is reachable")
		if !h.DUT.Connected(cleanupCtx) {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
				s.Fatal("Failed to set power_state to reset: ", err)
			}
			if err := ms.FwScreenToNormalMode(ctx); err != nil {
				s.Fatal("Failed while moving from firmware screen to normal mode: ", err)
			}
		}

		s.Log("Reverting the 'dev mode disable' state on DUT at the end of test")
		if err := setFWMP(cleanupCtx, "0"); err != nil {
			s.Fatal("Failed while taking ownership and setting flags at the end of test: ", err)
		}

		// Save the firmware log file for upload to Testhaus at the end of the test.
		output, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		destPath := filepath.Join(s.OutDir(), "firmware.log")
		if err := ioutil.WriteFile(destPath, []byte(output), 0666); err != nil {
			s.Fatal("Failed to write firmware log: ", err)
		}
	}(cleanupCtx)

	// Set DUT in "dev mode disable" state by setting TPM flags to "0x1".
	if err := setFWMP(ctx, "1"); err != nil {
		s.Fatal("Failed while taking ownership and setting flags: ", err)
	}

	ownershipData, err := s.DUT().Conn().CommandContext(ctx, "hwsec-ownership-id", "id").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get ownership ID: ", err)
	}
	ownershipID := strings.TrimSpace(string(ownershipData))

	// When dev mode is disabled by FWMP, DUT is expected to boot into normal mode.
	rebootCtx, cancelRebootCtx := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelRebootCtx()

	var opts []firmware.ModeSwitchOption
	opts = append(opts, firmware.SkipModeCheckAfterReboot)
	if err := ms.ModeAwareReboot(rebootCtx, firmware.ColdReset, opts...); err != nil {
		// When dev mode got disabled by FWMP, pressing the ENTER key would be
		// required on DUTs with RecButtonDevSwitch, for example chromeboxes,
		// to free them from the 'confirm returning to secure mode' screen.
		if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) && h.Config.RecButtonDevSwitch {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				s.Log("Pressing ENTER key")
				if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
					errors.Wrap(err, "failed to press enter key")
				}

				recButtonCtx, cancelRecButtonCtx := context.WithTimeout(ctx, 1*time.Minute)
				defer cancelRecButtonCtx()
				if err := h.WaitConnect(recButtonCtx); err != nil {
					return errors.Wrap(err, "failed to reconnect after pressing ENTER key")
				}
				return nil
			}, &testing.PollOptions{Timeout: 4 * time.Minute}); err != nil {
				s.Fatal("While trying to reboot DUT with dev mode disabled: ", err)
			}
		} else {
			s.Fatal("Unexpected error occurred while attempting to boot DUT: ", err)
		}
	}

	// Check boot mode after the reboot.
	mode, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		s.Fatal("Failed to check boot mode: ", err)
	}
	if mode == fwCommon.BootModeDev {
		// Read the firmware log.
		out, err := h.Reporter.CatFile(ctx, "/sys/firmware/log")
		if err != nil {
			s.Fatal("Failed to read firmware log: ", err)
		}
		re := regexp.MustCompile(`[vboot_draw_|vb2ex_display_ui|ui_display].*screen=0x([0-9a-f]+),?` +
			`\s+locale=\d+,?\s+(selected_item=\d+|selected_index=\d+)?`)
		screensVisited := re.FindAllStringSubmatch(out, -1)
		// Map firmware screen ids to respective names for readability.
		var fwScreenNames = map[fwCommon.FwScreenID]string{
			// Legacy clamshell UI.
			fwCommon.LegacyBlank:            "blank",
			fwCommon.LegacyDeveloperWarning: "developerWarning",
			fwCommon.LegacyDeveloperToNorm:  "developerToNorm",
			// Legacy menu UI.
			fwCommon.LegacyDeveloperWarningMenu: "developerWarningMenu",
			fwCommon.LegacyDeveloperMenu:        "developerMenu",
			fwCommon.LegacyDeveloperToNormMenu:  "developerToNormMenu",
			fwCommon.LegacyLanguagesMenu:        "languagesMenu",
			// Menu UI.
			fwCommon.AdvancedOptions: "advancedOptions",
			fwCommon.LanguageSelect:  "languageSelect",
			fwCommon.DebugInfo:       "debugInfo",
			fwCommon.FirmwareLog:     "firmwareLog",
			fwCommon.DeveloperMode:   "developerMode",
			fwCommon.DeveloperToNorm: "developerToNorm",
		}
		var foundScreens []string
		for _, screen := range screensVisited {
			if len(screen) != 3 {
				s.Fatal("Found unexpected matches: ", screen)
			}
			screenNumber, err := strconv.ParseInt(screen[1], 16, 64)
			if err != nil {
				s.Fatalf("Failed to parse screen number for %s: %v", screen[1], err)
			}
			if name, ok := fwScreenNames[fwCommon.FwScreenID(screenNumber)]; ok {
				foundScreens = append(foundScreens, fmt.Sprintf("%s, %s", name, screen[2]))
			} else {
				foundScreens = append(foundScreens, fmt.Sprintf("screen=0x%s, %s", screen[1], screen[2]))
			}
		}

		// Document the result of fwmp when DUT boots unexpectedly into dev mode.
		combinedOutput, err := s.DUT().Conn().CommandContext(ctx, "cryptohome", "--action=get_firmware_management_parameters").CombinedOutput(ssh.DumpLogOnError)
		if err != nil {
			s.Logf("Running 'cryptohome' on DUT failed: %v, and received: %s", err, combinedOutput)
		} else {
			s.Logf("Got current fwmp result: %s", combinedOutput)
		}

		s.Fatalf("DUT booted unexpectedly into dev mode after FWMP flags set to 0x1, and went through the following firmware screens: %s", strings.Join(foundScreens, "; "))
	}

	// Confirm TPM ownership changed.
	s.Log("Checking that TPM ownership changed at the end of the test")
	if err = s.DUT().Conn().CommandContext(ctx, "hwsec-ownership-id", "diff", "--id="+ownershipID).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("While checking TPM ownership: ", err)
	}
}
