// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// EcStress attempts to stress the EC in different dimensions to induce
// a crash. To achieve the performance and concurrency necessary to stress
// the EC, many of the standard APIs and interfaces cannot be used. Since
// not standard interfaces are used, this test can be unstable.

// Some meet devices don't support suspend. b/452039869#comment33
var suspendSkipModels = hwdep.SkipOnModel("intrepid", "genesis")

func init() {
	testing.AddTest(&testing.Test{
		Func: EcStress,
		Desc: "Stress EC to cause watchdog",
		Contacts: []string{
			"chromeos-faft@google.com",
			"robbarnes@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"chrome"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		Vars: []string{
			"firmware.EcStress.suspend",
			"firmware.EcStress.sensors",
			"firmware.EcStress.keyscan",
			"firmware.EcStress.flash",
			"firmware.EcStress.pd",
			"firmware.EcStress.period",
		},
		Timeout: time.Hour,
		Params: []testing.Param{
			{
				Name: "bare",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      false,
					sensors: false,
					suspend: false,
				},
			},
			{
				Name: "flash",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      false,
					sensors: false,
					suspend: false,
				},
			},
			{
				Name: "keyscan",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      false,
					sensors: false,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard()),
			},
			{
				Name: "pd",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      true,
					sensors: false,
					suspend: false,
				},
			},
			{
				Name: "sensors",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      false,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor()),
			},
			{
				Name: "suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      false,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(suspendSkipModels),
			},
			{
				Name: "flash_keyscan",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      false,
					sensors: false,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard()),
			},
			{
				Name: "flash_pd",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      true,
					sensors: false,
					suspend: false,
				},
			},
			{
				Name: "flash_sensors",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      false,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor()),
			},
			{
				Name: "flash_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      false,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(suspendSkipModels),
			},
			{
				Name: "keyscan_pd",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      true,
					sensors: false,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard()),
			},
			{
				Name: "keyscan_sensors",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      false,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor()),
			},
			{
				Name: "keyscan_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      false,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), suspendSkipModels),
			},
			{
				Name: "pd_sensors",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      true,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor()),
			},
			{
				Name: "pd_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      true,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(suspendSkipModels),
			},
			{
				Name: "sensors_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      false,
					sensors: true,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "flash_keyscan_pd",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      true,
					sensors: false,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard()),
			},
			{
				Name: "flash_keyscan_sensors",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      false,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor()),
			},
			{
				Name: "flash_keyscan_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      false,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), suspendSkipModels),
			},
			{
				Name: "flash_pd_sensors",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      true,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor()),
			},
			{
				Name: "flash_pd_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      true,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(suspendSkipModels),
			},
			{
				Name: "flash_sensors_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      false,
					sensors: true,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "keyscan_pd_sensors",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      true,
					sensors: true,
					suspend: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor()),
			},
			{
				Name: "keyscan_pd_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      true,
					sensors: false,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), suspendSkipModels),
			},
			{
				Name: "keyscan_sensors_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      false,
					sensors: true,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "pd_sensors_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: false,
					pd:      true,
					sensors: true,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "flash_keyscan_pd_sensors",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      true,
					sensors: true,
					suspend: false,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor()),
			},
			{
				Name: "flash_keyscan_pd_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      true,
					sensors: false,
					suspend: true,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), suspendSkipModels),
			},
			{
				Name: "flash_keyscan_sensors_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      false,
					sensors: true,
					suspend: true,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "flash_pd_sensors_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: false,
					pd:      true,
					sensors: true,
					suspend: true,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "keyscan_pd_sensors_suspend",
				Val: ecStressParams{
					flash:   false,
					keyscan: true,
					pd:      true,
					sensors: true,
					suspend: true,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "flash_keyscan_pd_sensors_suspend",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      true,
					sensors: true,
					suspend: true,
				},
				ExtraAttr:         []string{"group:firmware", "firmware_stress"},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor(), suspendSkipModels),
			},
			{
				Name: "all",
				Val: ecStressParams{
					flash:   true,
					keyscan: true,
					pd:      true,
					sensors: true,
					suspend: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Keyboard(), hwdep.MotionSensor(), suspendSkipModels),
			},
		},
	})
}

type ecStressParams struct {
	suspend bool
	sensors bool
	keyscan bool
	flash   bool
	pd      bool
}

type cancelfunc func() error

const defaultStressPeriod = 60 * time.Second
const timeoutPadding = 20 * time.Second
const iioBasePath = "/sys/bus/iio/devices"
const keyboardWakeupPath = "/sys/devices/platform/i8042/serio0/power/wakeup"

// errNoDeviceFound is returned by parser function when no device matches.
var errNoDeviceFound = errors.New("no Device found")

// errUnknownDeviceFound is returned by parser for unsupported devices, like
// lid angle or acpi-als light sensor.
var errUnknownDeviceFound = errors.New("unknown Device found")

// Sensor represents one sensor on the DUT.
type sensor struct {
	Path          string
	Name          string
	Location      string
	IioID         uint
	ID            uint
	Scale         float64
	MinFrequency  int
	MaxFrequency  int
	OldSysfsStyle bool
}

const (
	// Accel is an accelerometer sensor.
	accel string = "cros-ec-accel"
	// Gyro is a gyroscope sensor.
	gyro string = "cros-ec-gyro"
	// Mag is a magnetometer sensor.
	mag string = "cros-ec-mag"
	// Light is a light or proximity sensor.
	light string = "cros-ec-light"
	// Baro is a barometer.
	baro string = "cros-ec-baro"
	// Ring is a special sensor for ChromeOS that produces a stream of data from
	// all sensors on the DUT.
	ring string = "cros-ec-ring"
	// Activity is a special sensor for ChromeOS that produces several kind of
	// activity events by the data of other sensors.
	activity string = "cros-ec-activity"
)

var supportedSensors = []string{
	accel,
	gyro,
	mag,
	light,
}

const (
	// Base means that the sensor is located in the base of the DUT.
	base string = "accel-base"
	// Lid means that the sensor is located in the lid of the DUT.
	lid string = "accel-display"
	// Camera means that the sensor is located near the camera.
	camera string = "accel-camera"
	// None means that the sensor location is not known or not applicable.
	none string = "none"
)

var sensorNames = map[string]struct{}{
	accel:    {},
	baro:     {},
	gyro:     {},
	light:    {},
	mag:      {},
	ring:     {},
	activity: {},
}

var sensorLocations = map[string]struct{}{
	base:   {},
	lid:    {},
	camera: {},
}

var sensorLegacyLocationConverters = map[string]string{
	"base":   base,
	"lid":    lid,
	"camera": camera,
}

func getBoolVar(s *testing.State, varName string) *bool {
	valueStr, ok := s.Var(varName)
	if ok {
		if value, err := strconv.ParseBool(valueStr); err != nil {
			s.Fatalf("Invalid value for var %v: %v", varName, valueStr)
		} else {
			return &value
		}
	}
	return nil
}

func getIntVar(s *testing.State, varName string) *int {
	var value int
	valueStr, ok := s.Var(varName)
	if ok {
		value64, err := strconv.ParseInt(valueStr, 10, 32)
		if err != nil {
			s.Fatalf("Invalid value for var %v: %v", varName, valueStr)
		}
		value = int(value64)
		return &value
	}
	return nil
}

func startTask(ctx context.Context, timeout time.Duration, task, done func(context.Context) error) cancelfunc {
	errChan := make(chan error, 1)
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	go func() {
		defer close(errChan)
		for {
			select {
			case <-taskCtx.Done():
				if done != nil {
					errChan <- done(ctx)
				}
				return
			default:
				if task == nil {
					continue
				}
				if err := task(taskCtx); err != nil {
					// Ignore errors returned after taskCtx is done
					if taskCtx.Err() == nil {
						errChan <- err
						cancel()
					}
				}

			}
		}
	}()
	return func() error {
		cancel()
		return <-errChan
	}
}

func getDutFs(ctx context.Context, h *firmware.Helper) (*dutfs.Client, error) {
	if err := h.RequireRPCClient(ctx); err != nil || h.RPCClient == nil {
		return nil, errors.Wrap(err, "failed to connect rpc client on DUT")
	}
	return dutfs.NewClient(h.RPCClient.Conn), nil
}

// ****** Utilities for managing background processes on remote DUT *****

func startBackgroundProcess(ctx context.Context, h *firmware.Helper, cmd, outputFile string, timeout time.Duration) (cancelfunc, error) {
	testing.ContextLogf(ctx, "Starting background process with %v timeout: %q", timeout, cmd)
	wrappedCmd := fmt.Sprintf("{ nohup bash -c '%s' </dev/null &> %s & }; echo $!", cmd, outputFile)
	finalCmd := h.DUT.Conn().CommandContext(ctx, "bash", "-c", wrappedCmd)
	out, err := finalCmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to start background process")
	}
	outStr := strings.TrimSpace(string(out))
	pid, err := strconv.Atoi(outStr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse pid %s", outStr)
	}
	testing.ContextLogf(ctx, "Background process pid: %d", pid)

	// Background process killer runs in background on remote,
	// and kills the above background process after wall clock timeout.
	backgroundKillCmd := fmt.Sprintf("{ nohup bash -c 'declare -i END=$(date +%%s -d \"%v seconds\"); while [ $(date +%%s) -lt  $END ]; do sleep 0.1; done; kill -9 %d $(pgrep -P %d 2>/dev/null);' </dev/null &> /dev/null & };", timeout.Seconds(), pid, pid)
	if err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", backgroundKillCmd).Run(); err != nil {
		return nil, errors.Wrap(err, "failed to start background process killer")
	}

	return startTask(ctx, timeout, nil,
		func(backgroundCtx context.Context) error {
			if err := connectDut(ctx, h); err != nil {
				return errors.Wrapf(err, "failed to connect to DUT after finishing background process %d", pid)
			}
			if rebooted, err := dutRebooted(ctx, h); err != nil {
				return err
			} else if rebooted {
				// No need to kill background process if dut rebooted
				return nil
			}
			if err := killProcess(ctx, h, pid); err != nil {
				return errors.Wrapf(err, "failed to kill background process %d", pid)
			}
			return nil
		},
	), nil
}

func killProcess(ctx context.Context, h *firmware.Helper, pid int) error {
	testing.ContextLogf(ctx, "Killing background process %d", pid)
	if running, err := processIsRunning(ctx, h, pid); err != nil {
		return errors.Wrapf(err, "failed to check if process %d is running", pid)
	} else if !running {
		return nil
	}
	// Terminate parent shell and any child processes simultaneously to prevent orphan process creation.
	killCmd := fmt.Sprintf("kill -9 %d $(pgrep -P %d 2>/dev/null) 2>/dev/null", pid, pid)
	_ = h.DUT.Conn().CommandContext(ctx, "bash", "-c", killCmd).Run()

	if running, err := processIsRunning(ctx, h, pid); err != nil {
		return errors.Wrapf(err, "failed to check if process %d is running", pid)
	} else if !running {
		return nil
	}
	return errors.Errorf("failed to kill process %d", pid)
}

func processIsRunning(ctx context.Context, h *firmware.Helper, pid int) (bool, error) {
	err := h.DUT.Conn().CommandContext(ctx, "ps", "-p", strconv.Itoa(pid)).Run()
	if err == nil {
		return true, nil
	}
	if err.Error() == "Process exited with status 1" {
		return false, nil
	}
	return false, err
}

// ****** Sensor Utility Functions ******

func getSensors(ctx context.Context, h *firmware.Helper) ([]*sensor, error) {
	var ret []*sensor

	fs, err := getDutFs(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get fs client while getting sensors")
	}

	// Some systems will not have any iio devices; this case should not be an error.
	if _, err := fs.Stat(ctx, iioBasePath); os.IsNotExist(err) {
		return ret, nil
	}

	files, err := fs.ReadDir(ctx, iioBasePath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		sensor, err := parseSensor(ctx, fs, file.Name())
		if err != nil {
			if !errors.Is(err, errNoDeviceFound) && !errors.Is(err, errUnknownDeviceFound) {
				testing.ContextLogf(ctx, "Parsing sensor %s FAILED: %+v", file.Name(), err)
			}
			continue
		}
		testing.ContextLogf(ctx, "Found sensor %s with max frequency %d", sensor.Name, sensor.MaxFrequency)
		ret = append(ret, sensor)
	}

	return ret, nil
}

// ReadAttr reads the device's attr file and returns the value.
func (d *sensor) ReadAttr(ctx context.Context, fs *dutfs.Client, attr string) (string, error) {
	a, err := fs.ReadFile(ctx, filepath.Join(iioBasePath, d.Path, attr))
	if err != nil {
		return "", errors.Wrapf(err, "error reading attribute %q of %v", attr, d.Path)
	}
	return strings.TrimSpace(string(a)), nil
}

// parseSensor reads the sysfs directory at iioBasePath/devName and returns a
// Sensor if it is a valid EC sensor.
func parseSensor(ctx context.Context, fs *dutfs.Client, devName string) (*sensor, error) {
	var sensor sensor
	var location string = none
	var name string
	var id, minFreq, maxFreq int
	var scale float64
	var zeroInt, zeroFrac, minInt, minFrac, maxInt, maxFrac int

	if _, err := fmt.Sscanf(devName, "iio:device%d", &sensor.IioID); err != nil {
		// Could be a trigger, skip.
		return nil, errNoDeviceFound
	}

	sensor.Path = devName

	name, err := sensor.ReadAttr(ctx, fs, "name")
	if err != nil {
		return nil, errors.Wrap(err, "sensor has no name")
	}
	if _, ok := sensorNames[name]; !ok {
		return nil, errUnknownDeviceFound
	}

	if location, err := sensor.ReadAttr(ctx, fs, "label"); err == nil {
		if _, ok := sensorLocations[location]; !ok {
			return nil, errors.Errorf("unknown sensor label %q", location)
		}
	} else if loc, err := sensor.ReadAttr(ctx, fs, "location"); err == nil {
		// |location| attribute is for older kernels.
		var ok bool
		_, ok = sensorLegacyLocationConverters[loc]
		if !ok {
			return nil, errors.Errorf("unknown sensor location %q", loc)
		}
	}

	s, err := sensor.ReadAttr(ctx, fs, "scale")
	if err == nil {
		scale, err = strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid scale %q", s)
		}
	}

	i, err := sensor.ReadAttr(ctx, fs, "id")
	if err == nil {
		id, err = strconv.Atoi(i)
		if err != nil {
			return nil, errors.Wrapf(err, "bad sensor id %q", i)
		}

		if id < 0 {
			return nil, errors.Errorf("invalid sensor id %v", id)
		}
	}

	_, err = sensor.ReadAttr(ctx, fs, "frequency")
	sensor.OldSysfsStyle = err == nil

	if sensor.OldSysfsStyle {
		f, err := sensor.ReadAttr(ctx, fs, "min_frequency")
		if err == nil {
			minFreq, err = strconv.Atoi(f)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid min frequency %q", f)
			}
		}

		f, err = sensor.ReadAttr(ctx, fs, "max_frequency")
		if err == nil {
			maxFreq, err = strconv.Atoi(f)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid max frequency %q", f)
			}
		}
	} else {
		f, err := sensor.ReadAttr(ctx, fs, "sampling_frequency_available")
		if err == nil {
			_, err = fmt.Sscanf(f, "%d.%06d %d.%06d %d.%06d",
				&zeroInt, &zeroFrac, &minInt, &minFrac, &maxInt, &maxFrac)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid frequency range %q", f)
			}
			if zeroInt != 0 || zeroFrac != 0 {
				return nil, errors.Wrapf(err, "frequency range must start with 0 %q", f)
			}
			// In this code, frequency unit is mHz. iio now reports frequency in Hz with
			// 6 digits of precision.
			// So 12.5Hz will be printed 12.500000.
			// Int will be 12, Frac 500000.
			minFreq = minInt*1000 + minFrac/1000
			maxFreq = maxInt*1000 + maxFrac/1000
		}
	}

	sensor.Name = name
	sensor.Location = location
	sensor.ID = uint(id)
	sensor.Scale = scale
	sensor.MinFrequency = minFreq
	sensor.MaxFrequency = maxFreq

	return &sensor, nil
}

func startSensorStressTask(ctx context.Context, h *firmware.Helper, sn *sensor, timeout time.Duration) (cancelfunc, error) {

	testing.ContextLog(ctx, "Starting sensor stress task")
	channels := "timestamp"
	if sn.Name == accel {
		channels += " accel_x accel_y accel_z"
	} else if sn.Name == gyro {
		channels += " anglvel_x anglvel_y anglvel_z"
	} else if sn.Name == mag {
		channels += " magn_x magn_y magn_z"
	} else if sn.Name == light {
		channels += " illuminance"
	} else if sn.Name == ring {
		return nil, errors.New("Kernel must be compiled with USE=iioservice")
	} else {
		// This sensor is not supported by this test, skip.
		return nil, errors.Errorf("Sensor %s is not supported", sn.Name)
	}

	iioserviceCmd := fmt.Sprintf("iioservice_simpleclient --frequency=%d.%03d --channels=%s --device_id=%d --samples=%d",
		sn.MaxFrequency/1000, sn.MaxFrequency%1000, channels, sn.IioID, int(timeout.Seconds()*float64(sn.MaxFrequency)/1000))
	return startBackgroundProcess(ctx, h, iioserviceCmd, fmt.Sprintf("/tmp/%s.out", sn.Path), timeout)
}

// ****** Keyboard Utility Functions ******

func enableKeyboardWakeup(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Enabling keyboard wakeup")
	fs, err := getDutFs(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get fs client while enabling keyboard wakeup")
	}
	return fs.WriteFile(ctx, keyboardWakeupPath, []byte("enabled"), 0644)
}

func disableKeyboardWakeup(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Disabling keyboard wakeup")
	fs, err := getDutFs(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get fs client while disabling keyboard wakeup")
	}
	return fs.WriteFile(ctx, keyboardWakeupPath, []byte("disabled"), 0644)
}

// pressKey presses and holds the 'h' key
// It doesn't matter which key is pressed for these tests
func pressKey(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Pressing key")
	if err := h.Servo.RunECCommand(ctx, "kbpress 3 2 1"); err != nil {
		return errors.Wrap(err, "failed to press key")
	}
	return nil
}

// releaseKey releases the 'h' key
func releaseKey(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Releasing key")
	if err := h.Servo.RunECCommand(ctx, "kbpress 3 2 0"); err != nil {
		return errors.Wrap(err, "failed to release key")
	}
	return nil
}

func wakeWithUsbKeywboard(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.On); err != nil {
		return errors.Wrapf(err, "failed to set %q to %q with servo", servo.USBKeyboard, servo.On)
	}
	testing.ContextLog(ctx, "Pressing enter key with USB keyboard to wake DUT")
	if err := h.Servo.KeypressWithDuration(ctx, servo.USBEnter, servo.DurPress); err != nil {
		return errors.Wrap(err, "unable to send USB enter key press from servo")
	}
	if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.Off); err != nil {
		return errors.Wrapf(err, "failed to set %q to %q with servo", servo.USBKeyboard, servo.Off)
	}
	return nil
}

// ****** Flash Utility Functions ******

func startFlashStressTask(ctx context.Context, h *firmware.Helper, timeout time.Duration) (cancelfunc, error) {
	testing.ContextLog(ctx, "Starting Flash Stress Task")
	const logOutputPath = "/tmp/ec_stress_flash_read.log"
	ec := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)
	flashSize, err := ec.FlashSize(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get EC flash size")
	}
	if flashSize <= 0 {
		return nil, errors.Errorf("flash size %d is invalid", flashSize)
	}
	// Limit read size per iteration so reads complete within seconds and log "done.",
	// preventing premature SIGKILL termination mid-read on large flash sizes.
	readSize := 256 * 1024
	if flashSize < readSize {
		readSize = flashSize
	}
	cmd := fmt.Sprintf("while true; do ectool flashread 0 %d /dev/null; done;", readSize)
	remoteFlashStressCancel, err := startBackgroundProcess(ctx, h, cmd, logOutputPath, timeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start flash stress task")
	}

	return startTask(ctx, timeout,
		nil,
		func(flashCtx context.Context) error {
			testing.ContextLog(ctx, "Flash Stress Task Done")
			if err := connectDut(ctx, h); err != nil {
				return errors.Wrap(err, "failed to connect to DUT after finishing flash stress task")
			}
			if rebooted, err := dutRebooted(ctx, h); err != nil {
				return err
			} else if rebooted {
				// No cleanup or checks needed if dut rebooted
				return nil
			}
			if err := remoteFlashStressCancel(); err != nil {
				return errors.Wrap(err, "failed to stop flash stress task")
			}
			fs, err := getDutFs(ctx, h)
			if err != nil {
				return errors.Wrap(err, "failed to get fs client while stopping flash task")
			}
			logOutput, err := fs.ReadFile(ctx, logOutputPath)
			if err != nil {
				return errors.Wrap(err, "failed to stop flash stress task")
			}
			// "done." will be printed after every complete read of the flash
			flashReadCount := strings.Count(string(logOutput), "done.")
			testing.ContextLog(ctx, "Flash Stress Cycle Count: ", flashReadCount)
			if flashReadCount < 1 {
				return errors.New("failed to complete a full flash read")
			}
			return nil
		},
	), nil
}

// ****** Suspend Utility Functions ******

// getSuspendTargetCmd determines the correct command to trigger suspend based on system configuration.
// It prefers S0ix (freeze) if supported and configured, and falls back to S3 (deep) otherwise.
func getSuspendTargetCmd(ctx context.Context, h *firmware.Helper) (string, error) {
	// 1. Check if S0ix (freeze) is supported by the kernel.
	hasS0ix := false
	if stateBytes, err := h.DUT.Conn().CommandContext(ctx, "cat", "/sys/power/state").Output(); err == nil {
		hasS0ix = strings.Contains(string(stateBytes), "freeze")
	}

	// 2. Check if S3 (deep) is supported by the kernel.
	hasS3 := false
	if memSleepBytes, err := h.DUT.Conn().CommandContext(ctx, "cat", "/sys/power/mem_sleep").Output(); err == nil {
		hasS3 = strings.Contains(string(memSleepBytes), "deep")
	}

	// 3. Check powerd configuration for suspend_to_idle preference.
	// We check R/W prefs, board-specific R/O prefs, common R/O prefs, and unified config (cros_config).
	prefCmd := `cat /var/lib/power_manager/suspend_to_idle 2>/dev/null || ` +
		`cat /usr/share/power_manager/board_specific/suspend_to_idle 2>/dev/null || ` +
		`cat /usr/share/power_manager/suspend_to_idle 2>/dev/null || ` +
		`cat /run/chromeos-config/v1/power/suspend-to-idle 2>/dev/null || ` +
		`echo 0`

	outBytes, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", prefCmd).Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to read powerd suspend_to_idle preference")
	}
	pref := strings.TrimSpace(string(outBytes))
	suspendToIdlePref := (pref == "1")

	testing.ContextLogf(ctx, "Suspend detection: S0ix_supported=%t, S3_supported=%t, suspend_to_idle_pref=%t", hasS0ix, hasS3, suspendToIdlePref)

	// Decision logic:
	// Use S0ix if S0ix is supported AND suspend_to_idle pref is explicitly enabled.
	if hasS0ix && suspendToIdlePref {
		testing.ContextLog(ctx, "Selecting S0ix (freeze) for suspend")
		return "echo freeze > /sys/power/state", nil
	}

	// Otherwise, fall back to S3 if supported.
	if hasS3 {
		testing.ContextLog(ctx, "Selecting S3 (deep) for suspend")
		// Ensure 'deep' is selected in mem_sleep before echoing 'mem' to state
		return "echo deep > /sys/power/mem_sleep && echo mem > /sys/power/state", nil
	}

	// Ultimate fallback to S0ix if S3 is not supported but S0ix is.
	if hasS0ix {
		testing.ContextLog(ctx, "S3 unsupported, falling back to S0ix (freeze)")
		return "echo freeze > /sys/power/state", nil
	}

	return "", errors.New("neither S0ix (freeze) nor S3 (deep) suspend states are supported by this system")
}

func getKernelSuspendCount(ctx context.Context, h *firmware.Helper) (int, error) {
	out, err := h.DUT.Conn().CommandContext(ctx, "cat", "/sys/power/suspend_stats/success").Output()
	if err != nil {
		return 0, errors.Wrap(err, "failed to read /sys/power/suspend_stats/success")
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse suspend count %q", string(out))
	}
	return count, nil
}

func startSuspendStressTask(ctx context.Context, h *firmware.Helper, timeout, wakePeriod, suspendPeriod time.Duration) (cancelfunc, error) {
	testing.ContextLogf(ctx, "Starting Suspend Stress Task with wakePeriod=%v, suspendPeriod=%v", wakePeriod, suspendPeriod)

	suspendTargetCmd, err := getSuspendTargetCmd(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine suspend command")
	}

	initialCount, err := getKernelSuspendCount(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get initial suspend count")
	}

	// powerd_dbus_suspend is not used because user input from the keyboard prevents suspend
	cmd := fmt.Sprintf("while true; do sleep %v; echo 0 > /sys/class/rtc/rtc0/wakealarm; echo +%v > /sys/class/rtc/rtc0/wakealarm; %s; done;",
		wakePeriod.Seconds(), suspendPeriod.Seconds(), suspendTargetCmd)
	remoteSuspendStressCancel, err := startBackgroundProcess(ctx, h, cmd, "/tmp/ec_suspend_stress.out", timeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start suspend stress task")
	}

	var (
		once   sync.Once
		retErr error
	)
	return func() error {
		once.Do(func() {
			if err := remoteSuspendStressCancel(); err != nil {
				retErr = err
				return
			}
			if rebooted, err := dutRebooted(ctx, h); err != nil {
				retErr = err
				return
			} else if rebooted {
				// No cleanup or checks needed if dut rebooted
				return
			}

			finalCount, err := getKernelSuspendCount(ctx, h)
			if err != nil {
				retErr = errors.Wrap(err, "failed to get final suspend count")
				return
			}

			suspendCount := finalCount - initialCount
			testing.ContextLogf(ctx, "Suspend Stress Task Done; %d suspend cycles", suspendCount)
			if suspendCount < 1 {
				retErr = errors.New("DUT never suspended")
				return
			}
		})
		return retErr
	}, nil
}

func startPdStressTask(ctx context.Context, h *firmware.Helper, timeout time.Duration) (cancelfunc, error) {
	testing.ContextLog(ctx, "Starting PD Stress Task")
	// Enabling dual role port (DRP) will cause more stress on EC PD stack
	originalServoDualRoleState, err := h.Servo.ServoGetDualRoleState(ctx)
	if err != nil {
		originalServoDualRoleState = ""
		testing.ContextLog(ctx, "Failed to get DRP state on Servo")
	} else if originalServoDualRoleState != servo.USBPdDualRoleOn {
		if err := h.Servo.ServoSetDualRole(ctx, servo.USBPdDualRoleOn); err != nil {
			testing.ContextLog(ctx, "Failed to enable DRP on Servo")
		}
	}

	// GoBigSleepLint: Setting DRP on ServoV4 ('usbc_action drp') triggers reconnect
	// Wait some time to ensure that no operation will occur during test
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to sleep")
	}

	if err := h.Servo.SetString(ctx, "servo_uart_regexp", "None"); err != nil {
		return nil, errors.Wrap(err, "falied to clear Servo UART Regexp")
	}

	var disconnectCount = 0
	return startTask(ctx, timeout,
		func(pdCtx context.Context) error {
			const repeatCount = 20
			const sleepPeriod = 0.05
			if err := h.ServoProxy.RunCommandQuiet(pdCtx, true, "dut-control", "servo_uart_cmd:fakedisconnect 0 0", fmt.Sprintf("sleep:%v", sleepPeriod), fmt.Sprintf("--repeat=%v", repeatCount), fmt.Sprintf("--port=%d", h.ServoProxy.GetPort())); err != nil {
				return err
			}
			disconnectCount += repeatCount
			testing.ContextLogf(ctx, "PD Disconnect #%v", disconnectCount)
			// GoBigSleepLint: Allow PD port to complete attach negotiation and trigger HOOK_AC_CHANGE / charger I2C bursts
			return testing.Sleep(pdCtx, 2*time.Second)
		},
		func(pdCtx context.Context) error {
			testing.ContextLogf(ctx, "PD Stress Task Done; %v Disconnects", disconnectCount)
			if originalServoDualRoleState != "" {
				if err := h.Servo.ServoSetDualRole(ctx, originalServoDualRoleState); err != nil {
					testing.ContextLog(ctx, "Failed to restore DRP on Servo")
				}
			}
			return nil
		},
	), nil
}

func getParams(s *testing.State) ecStressParams {
	params := s.Param().(ecStressParams)
	if val := getBoolVar(s, "firmware.EcStress.keyscan"); val != nil {
		params.keyscan = *val
	}
	if val := getBoolVar(s, "firmware.EcStress.suspend"); val != nil {
		params.suspend = *val
	}
	if val := getBoolVar(s, "firmware.EcStress.flash"); val != nil {
		params.flash = *val
	}
	if val := getBoolVar(s, "firmware.EcStress.sensors"); val != nil {
		params.sensors = *val
	}
	if val := getBoolVar(s, "firmware.EcStress.pd"); val != nil {
		params.pd = *val
	}
	return params
}

var connectDutMutex sync.Mutex

func disconnectDut(ctx context.Context, h *firmware.Helper) error {

	// Calling connectDut/disconnectDut from various go routines can cause issues
	// Enforce one at a time
	connectDutMutex.Lock()
	defer connectDutMutex.Unlock()

	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to close rpc connection")
	}
	if err := h.DUT.Disconnect(ctx); err != nil {
		return errors.Wrap(err, "failed to disconnect DUT")
	}
	return nil
}

func connectDut(ctx context.Context, h *firmware.Helper) error {

	// Calling connectDut/disconnectDut from various go routines can cause issues
	// Enforce one at a time
	connectDutMutex.Lock()
	defer connectDutMutex.Unlock()

	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to servo")
	}

	state, err := h.Servo.GetECSystemPowerState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get EC power state")
	}
	if state != "S0" {
		testing.ContextLogf(ctx, "DUT is power sate %v, attempting to wake", state)
		wakeWithUsbKeywboard(ctx, h)
		if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
			return errors.Wrap(err, "failed to wake DUT")
		}
	}

	if h.DUT.Connected(ctx) {
		if err := h.RequireRPCClient(ctx); err != nil {
			return errors.Wrap(err, "failed to connect rpc client on DUT")
		}
		return nil
	}

	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to close rpc connection")
	}

	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for connect to DUT")
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "failed to connect rpc client on DUT")
	}

	return nil
}

func dumpParams(ctx context.Context, params ecStressParams, period time.Duration) {
	testing.ContextLogf(ctx, "Stress Keyscan:\t%v", params.keyscan)
	testing.ContextLogf(ctx, "Stress Flash:\t%v", params.flash)
	testing.ContextLogf(ctx, "Stress PD:\t\t%v", params.pd)
	testing.ContextLogf(ctx, "Stress Sensors:\t%v", params.sensors)
	testing.ContextLogf(ctx, "Stress Suspend:\t%v", params.suspend)
	testing.ContextLogf(ctx, "Stress Period:\t%v", period)
}

var originalBootID string

func dutRebooted(ctx context.Context, h *firmware.Helper) (bool, error) {
	newBootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get boot id")
	}
	return originalBootID != newBootID, nil
}

func EcStress(ctx context.Context, s *testing.State) {
	var stressPeriod = defaultStressPeriod
	var err error
	if val := getIntVar(s, "firmware.EcStress.period"); val != nil {
		stressPeriod = time.Duration(*val) * time.Second
	}
	params := getParams(s)
	dumpParams(ctx, params, stressPeriod)

	h := s.FixtValue().(*fixture.Value).Helper

	if err := connectDut(ctx, h); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove ccd watchdog: ", err)
	}

	// Collect the current boot ID to detect reboots
	if bootID, err := h.Reporter.BootID(ctx); err != nil {
		s.Fatal("Failed to get boot id: ", err)
	} else {
		originalBootID = bootID
	}

	var sensorStressTasksCancel []cancelfunc
	if params.sensors {
		// Start a sensor stress task for each sesnor
		sensors, err := getSensors(ctx, h)
		if err != nil {
			s.Fatal("Failed getting sensors on DUT: ", err)
		}
		if len(sensors) < 1 {
			s.Fatal("Failed to find any sensors")
		}
		for _, sn := range sensors {
			if !slices.Contains(supportedSensors, sn.Name) {
				s.Logf("Skip unsupported sensor %s", sn.Name)
				continue
			}
			cancel, err := startSensorStressTask(ctx, h, sn, stressPeriod+timeoutPadding)
			if err != nil {
				s.Fatalf("Failed to start stress task for sensor %s: %v", sn.Name, err)
			}
			defer cancel()
			sensorStressTasksCancel = append(sensorStressTasksCancel, cancel)
		}
	}

	var flashStressTaskCancel cancelfunc
	if params.flash {
		flashStressTaskCancel, err = startFlashStressTask(ctx, h, stressPeriod+timeoutPadding)
		if err != nil {
			s.Fatal("Failed to start flash stress task: ", err)
		}
		defer flashStressTaskCancel()
	}

	var suspendStressTaskCancel cancelfunc
	if params.suspend {
		if params.keyscan {
			// Keyboard must be suppressed as a wake source
			if err := disableKeyboardWakeup(ctx, h); err != nil {
				s.Log("Failed to disable keyboard wakeup source: ", err)
			} else {
				defer enableKeyboardWakeup(ctx, h)
			}
		}
		// 12s wake / 4s suspend (75% S0 duty cycle) allows AP-driven stress
		// tasks (e.g. flash and sensors) enough run time while still
		// exercising suspend and resume transitions.
		suspendStressTaskCancel, err = startSuspendStressTask(ctx, h, stressPeriod+timeoutPadding, time.Second*12, time.Second*4)
		if err != nil {
			s.Fatal("Failed to start suspend stress task: ", err)
		}
		defer suspendStressTaskCancel()
		// Suspend stress causes SSH to disconnect
		disconnectDut(ctx, h)
	}

	if params.keyscan {
		// Press and hold key
		pressKey(ctx, h)
		defer releaseKey(ctx, h)
	}

	var pdStressTaskCancel cancelfunc
	if params.pd {
		// GoBigSleepLint: PD stress breaks SSH connection, wait a second to make sure previous requests are done
		if err := testing.Sleep(ctx, time.Second*1); err != nil {
			s.Fatal("Failed to sleep during stress period: ", err)
		}
		// PD stress causes SSH to disconnect, close cleanly before starting
		disconnectDut(ctx, h)
		pdStressTaskCancel, err = startPdStressTask(ctx, h, stressPeriod+timeoutPadding)
		if err != nil {
			s.Fatal("Failed to start PD stress task: ", err)
		}
		defer pdStressTaskCancel()
	}

	s.Logf("Waiting %v seconds while EC is being stressed", stressPeriod.Seconds())
	// GoBigSleepLint: Wait while EC is being stressed
	if err := testing.Sleep(ctx, stressPeriod); err != nil {
		s.Fatal("Failed to sleep during stress period: ", err)
	}

	s.Log("Stress period over, cleaning up")

	if params.pd {
		if err := pdStressTaskCancel(); err != nil {
			s.Error("pd stress task failure: ", err)
		}
	}

	if params.keyscan {
		releaseKey(ctx, h)
	}

	// Remaining cleanup requires DUT to be connected
	if err := connectDut(ctx, h); err != nil {
		s.Fatal("Failed to connect to DUT after stress period: ", err)
	}

	if params.suspend {
		if err := suspendStressTaskCancel(); err != nil {
			s.Error("suspend stress task failure: ", err)
		}
	}

	if params.flash {
		if err := flashStressTaskCancel(); err != nil {
			s.Error("flash stress task failure: ", err)
		}
	}

	if params.sensors {
		for _, sensorTaskCancel := range sensorStressTasksCancel {
			if err := sensorTaskCancel(); err != nil {
				s.Error("sensor stress task failure: ", err)
			}
		}
	}

	if rebooted, err := dutRebooted(ctx, h); err != nil {
		s.Fatal("Failed to check if dut rebooted: ", err)
	} else if rebooted {
		s.Error("DUT rebooted unexpectedly")
	}

	// The firmware fixture will check for EC crashes and
	// fail the test if a crash is detected.
}
