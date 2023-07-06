// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"
	"io/ioutil"
	"time"

	"github.com/golang/protobuf/proto"

	pmpb "chromiumos/system_api/power_manager_proto"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     PowerdPolicies,
		Desc:     "Tests whether powerd correctly handles Wilco specific power policies received over D-Bus and writes correct data to exposed sysfs files by Dell EC drivers",
		Contacts: []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"vm_host", "wilco"},
	})
}

func PowerdPolicies(ctx context.Context, s *testing.State) {
	const powerdJobName = "powerd"
	_, _, oldPID, err := upstart.JobStatus(ctx, powerdJobName)
	if err != nil {
		s.Fatal("Failed to get powerd process ID: ", err)
	}
	defer func(ctx context.Context) {
		_, _, newPID, err := upstart.JobStatus(ctx, powerdJobName)
		if err != nil {
			s.Fatal("Failed to get powerd process ID: ", err)
		}
		if newPID != oldPID {
			s.Error("Unexpected powerd restart due to PID mismatch at the beginning of the test and now")
		}
	}(ctx)

	pm, err := power.NewPowerManager(ctx)
	if err != nil {
		s.Fatal("Failed to get new power manager: ", err)
	}

	var (
		// Battery Charge.
		chargeModeExpress = pmpb.PowerManagementPolicy_BatteryChargeMode_EXPRESS_CHARGE
		chargeModeCustom  = pmpb.PowerManagementPolicy_BatteryChargeMode_CUSTOM

		// Weekdays.
		weekDayMon = pmpb.PowerManagementPolicy_MONDAY
		weekDayTue = pmpb.PowerManagementPolicy_TUESDAY

		// Day times.
		time6_45 = pmpb.PowerManagementPolicy_DayTime{
			Hour: proto.Int32(6),
			// Valid values: 0, 15, 30, 45.
			Minute: proto.Int32(45),
		}
		time10_00 = pmpb.PowerManagementPolicy_DayTime{
			Hour:   proto.Int32(10),
			Minute: proto.Int32(0),
		}
		time18_45 = pmpb.PowerManagementPolicy_DayTime{
			Hour:   proto.Int32(18),
			Minute: proto.Int32(45),
		}
		time23_30 = pmpb.PowerManagementPolicy_DayTime{
			Hour:   proto.Int32(23),
			Minute: proto.Int32(30),
		}
	)

	type wantFile struct {
		filePath    string
		wantContent string
	}

	for _, tc := range []struct {
		name        string
		policyValue *pmpb.PowerManagementPolicy
		wantFiles   []wantFile
	}{
		// Boot on AC
		{
			"boot_on_ac_enable",
			&pmpb.PowerManagementPolicy{
				BootOnAc: proto.Bool(true),
			},
			// boot_on_ac file is write only, but we can verify that powerd does not crash.
			nil,
		},
		// USB Charge:
		{
			"usb_charge_disable",
			&pmpb.PowerManagementPolicy{
				UsbPowerShare: proto.Bool(false),
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/usb_charge",
					"0\n",
				},
			},
		},
		{
			"usb_charge_enable",
			&pmpb.PowerManagementPolicy{
				UsbPowerShare: proto.Bool(true),
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/usb_charge",
					"1\n",
				},
			},
		},
		// Battery Charge:
		{
			"battery_charge_express",
			&pmpb.PowerManagementPolicy{
				BatteryChargeMode: &pmpb.PowerManagementPolicy_BatteryChargeMode{
					Mode: &chargeModeExpress,
				},
			},
			[]wantFile{
				{
					"/sys/class/power_supply/wilco-charger/charge_type",
					"Fast\n",
				},
			},
		},
		{
			"battery_charge_custom1",
			&pmpb.PowerManagementPolicy{
				BatteryChargeMode: &pmpb.PowerManagementPolicy_BatteryChargeMode{
					Mode:              &chargeModeCustom,
					CustomChargeStart: proto.Int32(50),
					CustomChargeStop:  proto.Int32(80),
				},
			},
			[]wantFile{
				{
					"/sys/class/power_supply/wilco-charger/charge_type",
					"Custom\n",
				},
				{
					"/sys/class/power_supply/wilco-charger/charge_control_start_threshold",
					// The expected result might be different, because in the policy proto we use "display"
					// percentage (visible to the user), but in sysfs we write "actual" battery percentage.
					// Check BatteryPercentageConverter class for more details.
					"52\n",
				},
				{
					"/sys/class/power_supply/wilco-charger/charge_control_end_threshold",
					// Same.
					"81\n",
				},
			},
		},
		{
			"battery_charge_custom2",
			&pmpb.PowerManagementPolicy{
				BatteryChargeMode: &pmpb.PowerManagementPolicy_BatteryChargeMode{
					Mode:              &chargeModeCustom,
					CustomChargeStart: proto.Int32(60),
					CustomChargeStop:  proto.Int32(90),
				},
			},
			[]wantFile{
				{
					"/sys/class/power_supply/wilco-charger/charge_type",
					"Custom\n",
				},
				{
					"/sys/class/power_supply/wilco-charger/charge_control_start_threshold",
					// Same.
					"62\n",
				},
				{
					"/sys/class/power_supply/wilco-charger/charge_control_end_threshold",
					// Same.
					"90\n",
				},
			},
		},
		// Advanced Charging:
		{
			"advanced_charging_disable",
			&pmpb.PowerManagementPolicy{
				AdvancedBatteryChargeModeDayConfigs: nil,
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/enable",
					"0\n",
				},
			},
		},
		{
			"advanced_charging_enable1",
			&pmpb.PowerManagementPolicy{
				AdvancedBatteryChargeModeDayConfigs: []*pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
					&pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
						Day:             &weekDayMon,
						ChargeStartTime: &time10_00,
						ChargeEndTime:   &time18_45,
					},
					&pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
						Day:             &weekDayTue,
						ChargeStartTime: &time6_45,
						ChargeEndTime:   &time23_30,
					},
				},
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/enable",
					"1\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/monday",
					// Format is next: "{start_time} {diff_time}" where time format is "hh:mm".
					"10:00 08:45\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/tuesday",
					"06:45 16:45\n",
				},
			},
		},
		{
			"advanced_charging_enable2",
			&pmpb.PowerManagementPolicy{
				AdvancedBatteryChargeModeDayConfigs: []*pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
					&pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
						Day:             &weekDayMon,
						ChargeStartTime: &time6_45,
						ChargeEndTime:   &time23_30,
					},
					&pmpb.PowerManagementPolicy_AdvancedBatteryChargeModeDayConfig{
						Day:             &weekDayTue,
						ChargeStartTime: &time10_00,
						ChargeEndTime:   &time18_45,
					},
				},
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/enable",
					"1\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/monday",
					"06:45 16:45\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/tuesday",
					"10:00 08:45\n",
				},
			},
		},
		// Peak Shift:
		{
			"peak_shift_disable",
			&pmpb.PowerManagementPolicy{
				PeakShiftDayConfigs: nil,
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/enable",
					"0\n",
				},
			},
		},
		{
			"peak_shift_enable1",
			&pmpb.PowerManagementPolicy{
				PeakShiftBatteryPercentThreshold: proto.Int32(15),
				PeakShiftDayConfigs: []*pmpb.PowerManagementPolicy_PeakShiftDayConfig{
					&pmpb.PowerManagementPolicy_PeakShiftDayConfig{
						Day:             &weekDayMon,
						StartTime:       &time10_00,
						EndTime:         &time18_45,
						ChargeStartTime: &time23_30,
					},
					&pmpb.PowerManagementPolicy_PeakShiftDayConfig{
						Day:             &weekDayTue,
						StartTime:       &time6_45,
						EndTime:         &time23_30,
						ChargeStartTime: &time18_45,
					},
				},
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/enable",
					"1\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/battery_threshold",
					// Same.
					"18\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/monday",
					"10:00 18:45 23:30\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/tuesday",
					"06:45 23:30 18:45\n",
				},
			},
		},
		{
			"peak_shift_enable2",
			&pmpb.PowerManagementPolicy{
				PeakShiftBatteryPercentThreshold: proto.Int32(50),
				PeakShiftDayConfigs: []*pmpb.PowerManagementPolicy_PeakShiftDayConfig{
					&pmpb.PowerManagementPolicy_PeakShiftDayConfig{
						Day:             &weekDayMon,
						StartTime:       &time6_45,
						EndTime:         &time23_30,
						ChargeStartTime: &time18_45,
					},
					&pmpb.PowerManagementPolicy_PeakShiftDayConfig{
						Day:             &weekDayTue,
						StartTime:       &time10_00,
						EndTime:         &time18_45,
						ChargeStartTime: &time23_30,
					},
				},
			},
			[]wantFile{
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/enable",
					"1\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/battery_threshold",
					// Same.
					"52\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/monday",
					"06:45 23:30 18:45\n",
				},
				{
					"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/tuesday",
					"10:00 18:45 23:30\n",
				},
			},
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			if err := pm.SetPolicy(ctx, tc.policyValue); err != nil {
				s.Fatal("Failed to set power management policy to enable USB power share: ", err)
			}

			for _, wFile := range tc.wantFiles {
				if err := testing.Poll(ctx, func(ctx context.Context) error {
					bytes, err := ioutil.ReadFile(wFile.filePath)
					if err != nil {
						return testing.PollBreak(errors.Wrapf(err, "failed to read %s file", wFile.filePath))
					}
					if got, want := string(bytes), wFile.wantContent; got != want {
						return errors.Errorf("unexpected %s content: got %q, want %q", wFile.filePath, got, want)
					}
					return nil
				}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
					s.Error("Failed to wait expected file content: ", err)
				}
			}
		})
	}

}
