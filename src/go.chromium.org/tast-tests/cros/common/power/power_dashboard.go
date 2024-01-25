// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	pb "go.chromium.org/tast-tests/cros/common/power/powerpb"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// containEmpty is the helper function to check if there is an empty string among args.
func containEmpty(strs ...string) bool {
	for _, str := range strs {
		if str == "" {
			return true
		}
	}
	return false
}

// sortMetricsNumerically is the helper function to sort metrics in numerical order.
func sortMetricsNumerically(ctx context.Context, metrics []string) {
	sort.Slice(metrics, func(i, j int) bool {
		re := regexp.MustCompile("[0-9]+")
		iSlices := re.FindAllString(metrics[i], -1)
		jSlices := re.FindAllString(metrics[j], -1)

		var si int64
		var sj int64
		var err error

		if len(iSlices) >= 1 {
			si, err = strconv.ParseInt(iSlices[len(iSlices)-1], 10, 64)
			if err != nil {
				testing.ContextLog(ctx, "Failed to parse string to int: ", err)
			}
		}

		if len(jSlices) >= 1 {
			sj, err = strconv.ParseInt(jSlices[len(jSlices)-1], 10, 64)
			if err != nil {
				testing.ContextLog(ctx, "Failed to parse string to int: ", err)
			}
		}

		return si < sj
	})
}

// generateDashboardLink generates link to power and thermal dashboard.
func generateDashboardLink(powerLogDict map[string]interface{}) string {
	const hwidLinkStr = `
	<a href="http://goto.google.com/pdash-hwid?query={hwid}">
	  Link to hwid lookup.
	</a><br />
	`

	const pdashLinkStr = `
	<a href="http://chrome-power.appspot.com/dashboard?board={board}&test={test}&datetime={datetime}">
	  Link to power dashboard.
	</a><br />
	`

	const tdashLinkStr = `
	<a href="http://chrome-power.appspot.com/thermal_dashboard?note={note}">
	  Link to thermal dashboard.
	</a><br />
	`

	var board, test, hwid, note, datetime string
	var timeRaw time.Time

	if value, ok := powerLogDict["test"].(string); ok {
		test = value
	}
	if value, ok := powerLogDict["timestamp"].(int64); ok {
		timeRaw = time.Unix(value, 0).UTC()
	}
	datetime = fmt.Sprintf("%d%02d%02d%02d%02d", timeRaw.Year(), int(timeRaw.Month()), timeRaw.Day(), timeRaw.Hour(), timeRaw.Minute())
	if dutMap, ok := powerLogDict["dut"].(map[string]interface{}); ok {
		if value, ok := dutMap["board"].(string); ok {
			board = value
		}
		if value, ok := dutMap["note"].(string); ok {
			note = value
		}
		if skuMap, ok := dutMap["sku"].(map[string]interface{}); ok {
			if value, ok := skuMap["hwid"].(string); ok {
				hwid = value
			}
		}
	}

	htmlStr := `<!DOCTYPE html><html><body>`
	r := strings.NewReplacer("{hwid}", hwid, "{board}", board, "{test}", test, "{datetime}", datetime, "{note}", note)
	if !containEmpty(hwid) {
		htmlStr += r.Replace(hwidLinkStr)
	}
	if !containEmpty(board, test, datetime) {
		htmlStr += r.Replace(pdashLinkStr)
	}
	pattern := `ThermalQual.(full|lab).*`
	re := regexp.MustCompile(pattern)
	if re.MatchString(note) && !containEmpty(note) {
		htmlStr += r.Replace(tdashLinkStr)
	}
	htmlStr += `</body></html>`
	return htmlStr
}

// getMinutesBatteryLife calculates and returns the projected operating minutes.
func getMinutesBatteryLife(ctx context.Context, innerDataMap map[string][]float64, innerAverageMap map[string]float64, totalDurationSec float64, metrics *pb.OneTimeMetrics) (minutesBatteryLife float64) {
	errStr := func(metric string) string {
		e := "Cannot compute battery life in minutes and defaulting to 0 minute because "
		e += metric + " cannot be retrieved"
		return e
	}

	if metrics.BatteryChargeDesignSize == nil {
		testing.ContextLog(ctx, errStr("battery charge design size"))
		return 0
	}
	chargeFullDesign := metrics.GetBatteryChargeDesignSize()

	if metrics.BatteryChargeSize == nil {
		testing.ContextLog(ctx, errStr("battery charge"))
		return 0
	}
	chargeFull := metrics.GetBatteryChargeSize()

	if metrics.BatteryEnergySize == nil {
		testing.ContextLog(ctx, errStr("battery energy size"))
		return 0
	}
	energyFullDesign := metrics.GetBatteryEnergySize()

	if energyUsed, ok := innerAverageMap["discharge_mwh"]; ok && energyUsed > 0 && totalDurationSec > 0 {
		var lowBatteryShutdownPercent = 4.0
		if metrics.BatteryShutdownPercent == nil {
			es := "Low battery shut down percent cannot be retrieved "
			es += "while computing battery life in minutes and "
			es += "4% is used for approximation"
			testing.ContextLog(ctx, es)
		} else {
			lowBatteryShutdownPercent = metrics.GetBatteryShutdownPercent()
		}

		batSizeScale := 1 - lowBatteryShutdownPercent/100.0

		var chargeUsedInPercent float64
		if chargeValue, exist := innerDataMap["battery_percent"]; exist && len(chargeValue) > 1 {
			chargeUsedInPercent = chargeValue[len(chargeValue)-1] - chargeValue[0]
		}
		// For longer tests (> 1hr), charge (Ah) consumption is more accurate for calculating projected battery life.
		// For shorter tests (< 1hr), power integral (Wh) is more accurate for calculating projected battery life.
		const MinReasonableDuration = 3600
		if totalDurationSec > MinReasonableDuration && chargeUsedInPercent > 0 {
			// Use charge to project operation time when test run time > 1 hour.
			chargeRate := chargeUsedInPercent / (totalDurationSec / 60.0)
			minutesBatteryLife = batSizeScale * (chargeFullDesign / chargeFull) / chargeRate
		} else {
			// Use energy to project operation time when test run time < 1 hour.
			// Notice energyUsed is in mWh and battery (design) size is in Wh. energyRate is in Wh/min.
			energyRate := energyUsed / (totalDurationSec / 60.0) / 1000.0
			minutesBatteryLife = energyFullDesign * batSizeScale / energyRate
		}
	} else {
		// If energy used is 0 (test too short to cover valid samplings, test did not run on battery, ...):
		// Log that we will not calculate minutes_battery_life.
		testing.ContextLog(ctx, "Failed to calculate minutes_battery_life: 0 energy usage")
	}
	return minutesBatteryLife
}

// getNonSocSubsystemPowerData calculates and returns all subsystem power data other than SoC.
func getNonSocSubsystemPowerData(ctx context.Context,
	innerDataMap map[string][]float64,
	innerAverageMap map[string]float64) ([]float64, float64) {
	// System power data.
	systemPowerNumbers := innerDataMap["system"]
	// SoC power data.
	SocPowerNumbers := innerDataMap[Package0]
	// All subsystem(nonSoc) power data.
	nonSoCPowerNumbers := make([]float64, 0)

	for index := 0; index < len(systemPowerNumbers); index++ {
		nonSoCPowerNumbers = append(nonSoCPowerNumbers, systemPowerNumbers[index]-SocPowerNumbers[index])
	}

	nonSoCPowerAverage := innerAverageMap["system"] - innerAverageMap[Package0]
	return nonSoCPowerNumbers, nonSoCPowerAverage
}

// CreateSaveUploadPowerLog creates power log files and uploads them to dashboard.
// This function expects the unmodified perf values generated by the recorder.
func CreateSaveUploadPowerLog(ctx context.Context, outDir, testName, fileSuffix string, values *perf.Values, checkpoints *perf.Checkpoints, devInfo *pb.DeviceInfo, metrics *pb.OneTimeMetrics) (map[string]interface{}, error) {
	pwrLog, err := CreatePowerLogAndUpdatePerfValues(ctx, testName, values, checkpoints, devInfo, metrics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create power log and update perf")
	}

	if err := SavePowerLog(ctx, outDir, pwrLog, fileSuffix); err != nil {
		return nil, errors.Wrap(err, "failed to save power log")
	}

	if err := uploadToDashboard(ctx, pwrLog, ""); err != nil {
		return nil, errors.Wrap(err, "failed to upload power log to dashboard")
	}

	return pwrLog, nil
}

// CreatePowerLogAndUpdatePerfValues updates perf values with additional power
// metrics and converts perf values to power log.
// Perf values must be unmodified from the recorder, coming from perf.Timeline,
// or the upload (next step) may fail.
// Avoid calling this function more than once as perf values are updated during
// this process.
func CreatePowerLogAndUpdatePerfValues(ctx context.Context, testName string, values *perf.Values, checkpoints *perf.Checkpoints, devInfo *pb.DeviceInfo, metrics *pb.OneTimeMetrics) (map[string]interface{}, error) {
	// Conversion process also updates perf values in-place.
	pwrDict, startTs, err := convertPerfValuesToPowerDict(ctx, values, checkpoints, metrics)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert power perf values to power dictionary and start ts")
	}
	return createPowerLogFromPowerDict(ctx, testName, pwrDict, startTs, devInfo), nil
}

// convertPerfValuesToPowerDict converts raw performance metric values to power
// dictionary and start ts, and updates perf.Values.
func convertPerfValuesToPowerDict(ctx context.Context, values *perf.Values, checkpoints *perf.Checkpoints, metrics *pb.OneTimeMetrics) (map[string]interface{}, time.Time, error) {
	measurement := values.GetValues()
	if measurement == nil || len(measurement) == 0 {
		return nil, time.Time{}, errors.New("invalid power measurement")
	}

	powerDict := map[string]interface{}{
		// sample_count indicates how many time each metric has been collected.
		"sample_count": 0,
		// sample_duration is the time interval between two data points.
		"sample_duration": 0,
		// average is the mean of each metric.
		"average": nil,
		// data is all the data points collected for each metric.
		"data": nil,
		// type is a map from metric to type.
		"type": nil,
		// unit is a map from metric to unit.
		"unit": nil,
	}
	var start time.Time

	innerDataMap := make(map[string][]float64)
	innerAverageMap := make(map[string]float64)
	typeMap := make(map[string]string)
	unitMap := make(map[string]string)

	for metric, value := range measurement {
		// metric.Name is a string of "(prefix.)(metricType.)metricName"
		metricNameSlice := strings.Split(metric.Name, ".")
		var metricType string
		var metricName string

		size := len(metricNameSlice)

		// TODO: b/296507731 - Support prefixes in power recorder
		// if power recorder starts accepting prefixes, size conditions for
		// metricName/type should also be updated

		// metric.Name is guaranteed be non-empty string.
		metricName = metricNameSlice[size-1]

		// Ignore metrics unrelated to power. The power timeline can specify
		// a "snapshotsSkipped" metric that is reported when grace periods are
		// used for the perf.Timeline. Skip this metric, as it is mainly for
		// auditing, and is not power specific.
		if metricName == "snapshotsSkipped" {
			continue
		}

		// Four scenarios to be considered:
		// 1."system": used both by power and ARCVM team. To minimize interuption,
		// a power metric type is not assigned to it. Manually adding a metric type
		// for "system" because metric.Name doesn't include a metric type.
		// 2. "t": not a power metric, therefore a metric type isn't assigned and
		// it will not be included in the typeMap.
		// 3. size == 1: assign metricType to "other" when type is not provided.
		// 4. Assume format (metricType.)metricName from metricNameSlice.
		if metricName == "system" {
			metricType = "power"
		} else if metricName == "t" {
			innerDataMap[metricName] = value
			if !metric.HasStartTs {
				return nil, time.Time{}, errors.Errorf("metric %s does not have start ts", metric.Name)
			}
			if len(value) > 0 {
				start = metric.StartTs.Add(time.Duration(value[0] * float64(time.Second)))
			} else {
				testing.ContextLogf(ctx, "%q metric is an empty slice, so set power log timestamp to start of measurement instead of 1st data point timestamp", metricName)
				start = metric.StartTs
			}
			continue
		} else if size == 1 {
			metricType = "other"
		} else {
			metricType = metricNameSlice[0]
			metricName = strings.ToLower(strings.Join(metricNameSlice[1:size], "_"))
		}

		// Validate metricType.
		if _, ok := validMetricTypeMap[metricType]; !ok {
			return nil, time.Time{}, errors.Errorf("unexpected metric type %q for %q", metricType, metricName)
		}

		if len(metricType) != 0 {
			typeMap[metricName] = metricType
			unitMap[metricName] = metric.Unit
		}
		innerDataMap[metricName] = value

		sum := 0.0
		for _, num := range value {
			sum += num
		}

		var mean float64
		if len(value) != 0 {
			mean = sum / float64(len(value))
		}

		innerAverageMap[metricName] = mean
	}

	var totalDurationSec float64
	batteryLifeTested, ok := innerAverageMap[MinutesBatteryLifeTestedKey]
	if ok {
		totalDurationSec = batteryLifeTested * 60
	}

	// noTimeline indicates if the timeline is given in the collected metrics.
	noTimeline := false
	// TODO: b/316820383 - power dashboard should support multiple timelines.
	if value, ok := innerDataMap["t"]; ok {

		var sampleCount = len(value)
		powerDict["sample_count"] = sampleCount
		if sampleCount > 1 {
			firsTimestamp := value[0]
			lastTimestamp := value[sampleCount-1]
			powerDict["sample_duration"] = (lastTimestamp - firsTimestamp) / (float64(sampleCount) - 1)
		} else if sampleCount > 0 {
			// When sampleCount == 1, sample_duration may be wrong
			// and the power dashboard will only show 1 data point.
			lastTimestamp := value[sampleCount-1]
			powerDict["sample_duration"] = lastTimestamp / float64(sampleCount)
		}

		// checkpointsList is list of lists, where each list contains the
		// Checkpoint names that are on for the corresponding timestamp.
		checkpointsList, err := tagTimelineWithCheckpoints(values, checkpoints)
		if err != nil {
			return nil, time.Time{}, errors.Wrap(err, "unable to process the checkpoints for power_log.json")
		}
		if checkpointsList != nil {
			powerDict["checkpoint"] = checkpointsList
		}

	} else {
		// In Power Qual V2, the main test could ask to generate power log for
		// personas without giving a timeline. In this case, we want to generate
		// 2 samples for the key metric values, allowing a line chart instead of
		// a dot to be drawn in the dashboard.
		noTimeline = true
		powerDict["sample_count"] = 2
		powerDict["sample_duration"] = totalDurationSec
		// Since PowerQualV2 main test does not have perf.Timeline, it does not
		// record test start time. Use an approximate.
		start = time.Now()
	}

	minutesBatteryLife, batteryLifeOk := innerAverageMap[MinutesBatteryLifeKey]
	// Get battery life only when it is not available yet.
	if !batteryLifeOk && metrics != nil {
		minutesBatteryLife = getMinutesBatteryLife(ctx, innerDataMap, innerAverageMap, totalDurationSec, metrics)
		innerAverageMap[MinutesBatteryLifeKey] = minutesBatteryLife
	}
	values.Set(perf.Metric{
		Name:      GeneralPerfMetricType + MinutesBatteryLifeKey,
		Unit:      "minute",
		Direction: perf.BiggerIsBetter,
	}, minutesBatteryLife)
	innerDataMap[MinutesBatteryLifeKey] = getDataSamples(minutesBatteryLife, noTimeline)
	typeMap[MinutesBatteryLifeKey] = "perf"
	unitMap[MinutesBatteryLifeKey] = "minute"

	typeMap[MinutesBatteryLifeTestedKey] = "perf"
	innerDataMap[MinutesBatteryLifeTestedKey] = getDataSamples(batteryLifeTested, noTimeline)

	// Check if package-0 is collected first because `rapl` is not supported on all platforms.
	if _, ok := innerDataMap[Package0]; ok && len(innerDataMap["system"]) == len(innerDataMap[Package0]) {
		innerDataMap["non_SoC"], innerAverageMap["non_SoC"] = getNonSocSubsystemPowerData(ctx, innerDataMap, innerAverageMap)
		typeMap["non_SoC"] = "power"
		unitMap["non_SoC"] = PowerRelatedMetricTypeUnit
		values.Append(perf.Metric{
			Name:     PowerRelatedMetricType + "non_SoC",
			Unit:     PowerRelatedMetricTypeUnit,
			Multiple: true,
		}, innerDataMap["non_SoC"]...)
	}

	addPowerPerfValues(ctx, innerDataMap, innerAverageMap, typeMap, unitMap, metrics, noTimeline)

	powerDict["data"] = innerDataMap
	powerDict["average"] = innerAverageMap
	powerDict["type"] = typeMap
	powerDict["unit"] = unitMap
	return powerDict, start, nil
}

// addPowerPerfValues adds perf scalar to power log map.
func addPowerPerfValues(ctx context.Context, dataMap map[string][]float64, averageMap map[string]float64,
	typeMap, unitMap map[string]string, metrics *pb.OneTimeMetrics, noTimeline bool) {

	backlightNonlinearPercent, nonLinearOk := averageMap[BacklightPercentNonlinearKey]
	// Get it from metrics only if it is not yet available.
	if !nonLinearOk && metrics != nil {
		backlightNonlinearPercent = metrics.GetBacklightNonlinearPercent()
		averageMap[BacklightPercentNonlinearKey] = backlightNonlinearPercent
		nonLinearOk = true
	}
	if nonLinearOk {
		dataMap[BacklightPercentNonlinearKey] = getDataSamples(backlightNonlinearPercent, noTimeline)
		typeMap[BacklightPercentNonlinearKey] = "perf"
		unitMap[BacklightPercentNonlinearKey] = GeneralPerfMetricTypeUnit
	}

	backlightLinearPercent, linearOk := averageMap[BacklightPercentLinearKey]
	// Get it from metrics only if it is not yet available.
	if !linearOk && metrics != nil {
		backlightLinearPercent = metrics.GetBacklightLinearPercent()
		averageMap[BacklightPercentLinearKey] = backlightLinearPercent
		linearOk = true
	}
	if linearOk {
		dataMap[BacklightPercentLinearKey] = getDataSamples(backlightLinearPercent, noTimeline)
		typeMap[BacklightPercentLinearKey] = "perf"
		unitMap[BacklightPercentLinearKey] = GeneralPerfMetricTypeUnit
	}
}

// getDataSamples returns the data samples based on the twoSamples flag.
func getDataSamples(value float64, twoSamples bool) []float64 {
	if twoSamples {
		return []float64{value, value}
	}
	return []float64{value}
}

// createPowerLogFromPowerDict creates the power log dictionary from power dict
// and device info.
func createPowerLogFromPowerDict(ctx context.Context, testName string, powerDict map[string]interface{}, start time.Time, devInfo *pb.DeviceInfo) map[string]interface{} {
	// If perf.Timeline is used, timestamp will be the start ts of
	// perf.Timeline. The exception is PowerQualV2.
	// Since PowerQualV2 main test does not have perf.Timeline, it does not
	// record test start time. Use the timestamp at time of power_log creation
	// as an approximate.
	powerLogDict := map[string]interface{}{
		"format_version": 7,
		"timestamp":      float64(start.UnixNano()) / 1000000000.0,
		"test":           testName,
		"dut":            FormatDeviceInfoForPowerLog(devInfo),
		"power":          powerDict,
	}

	return powerLogDict
}

// SavePowerLog saves power log json and html to the specified directory.
func SavePowerLog(ctx context.Context, outDir string, pwrLog map[string]interface{}, fileSuffix string) error {
	if err := savePowerLogJSON(ctx, outDir, pwrLog, fileSuffix); err != nil {
		return errors.Wrap(err, "failed to generate power_log.json")
	}
	if err := savePowerLogHTML(ctx, outDir, pwrLog, fileSuffix); err != nil {
		return errors.Wrap(err, "failed to generate power_log.html")
	}
	return nil
}

// savePowerLogHTML saves the power log as a json file format.
func savePowerLogHTML(ctx context.Context, outDir string, powerLogDict map[string]interface{}, fileSuffix string) error {
	sampleCount := powerLogDict["power"].(map[string]interface{})["sample_count"].(int)

	if sampleCount <= 0 {
		return errors.Errorf("sampleCount is %d and should be bigger than 0", sampleCount)
	}

	htmlStr := generateDashboardLink(powerLogDict)

	sampleDuration := powerLogDict["power"].(map[string]interface{})["sample_duration"]
	powerLogDataMap := powerLogDict["power"].(map[string]interface{})["data"].(map[string][]float64)
	powerLogUnitMap := powerLogDict["power"].(map[string]interface{})["unit"].(map[string]string)
	powerLogTypeMap := powerLogDict["power"].(map[string]interface{})["type"].(map[string]string)

	// Generate a map from type to metric names.
	typeToMetricsMap := make(map[string][]string)

	for metric, metricType := range powerLogTypeMap {
		if _, ok := typeToMetricsMap[metricType]; !ok {
			typeToMetricsMap[metricType] = make([]string, 0)
		}

		// Exclude package-non-C0_C1 from typeToMetricMap and the cpupkg chart.
		// Ignore case by comparing everything in lower case.
		if strings.Contains(strings.ToLower(metric), "package-non-c0_c1") {
			continue
		}

		// For now, just visualize the aggregated cpu stats, not per-cpu stats.
		if metricType == "cpuidle" && !strings.HasPrefix(metric, "cpu-") {
			continue
		}

		typeToMetricsMap[metricType] = append(typeToMetricsMap[metricType], metric)
	}

	// Use a slice to maintain the metric type order and position "perf"
	// table to the bottom of html page.
	types := make([]string, 0, len(typeToMetricsMap))
	for t := range typeToMetricsMap {
		if t == "perf" {
			continue
		}
		types = append(types, t)
	}

	sort.Strings(types)
	// Add "perf" back to the end of the list so that "perf" always
	// stays at the bottom of power_log.html page.
	types = append(types, "perf")

	for metricType, metrics := range typeToMetricsMap {
		// Sort the following types in numerical order: cpu-C0, cpu-C1E, cpu-C6, cpu-C8, cpu-C10.
		if metricType == "cpuidle" || metricType == "cpupkg" {
			sortMetricsNumerically(ctx, metrics)
			continue
		}
		// Sort all other types in alphabetical order ignoring cases.
		sort.Slice(metrics, func(i, j int) bool {
			return strings.ToLower(metrics[i]) < strings.ToLower(metrics[j])
		})
	}

	rowIndentation := strings.Repeat(" ", 12)

	for _, metricType := range types {
		metrics := typeToMetricsMap[metricType]
		// Generate metric name string.
		// headerRowStr example:
		// "            ['time', 'zram_read_IOs', 'zram_IOs_in_flight', 'zram_write_IOs']".
		headerRow := append([]string{"time"}, typeToMetricsMap[metricType]...)
		headerRowStr := rowIndentation + "['" + strings.Join(headerRow, "', '") + "']"
		chartDataStrList := []string{headerRowStr}

		// Generate metric data string.
		for sampleIndex := 0; sampleIndex < sampleCount; sampleIndex++ {
			time := float64(sampleIndex) * sampleDuration.(float64)
			dataRow := []string{strconv.FormatFloat(time, 'g', -1, 64)}

			for _, metric := range metrics {
				// Most metrics are collected sample_count times, but there are exceptions,
				// such as "discharge_mwh", which is only collected once.
				if sampleIndex >= len(powerLogDataMap[metric]) {
					break
				}
				value := powerLogDataMap[metric][sampleIndex]
				dataRow = append(dataRow, strconv.FormatFloat(value, 'g', -1, 64))
			}

			// Scalar value.
			if sampleIndex == 1 && metricType == "perf" {
				break
			}

			// headerRowStr example:
			// "            ['time', 'zram_read_IOs', 'zram_IOs_in_flight', 'zram_write_IOs']".
			// Corresponding dataRowStr example:
			// "            [0, 296, 0, 1]".
			dataStr := rowIndentation + "[" + strings.Join(dataRow, ", ") + "]"
			chartDataStrList = append(chartDataStrList, dataStr)
		}

		chartDataStr := strings.Join(chartDataStrList, ",\n")

		var unit string
		if metricType == "perf" {
			unit = "point"
		} else {
			unit = powerLogUnitMap[metrics[0]]
		}

		r := strings.NewReplacer("{data}", chartDataStr, "{unit}", unit, "{type}", metricType)
		htmlStr += r.Replace(htmlChartStr)
	}

	filePath := path.Join(outDir, powerLogFileName+fileSuffix+".html")

	if err := os.WriteFile(filePath, []byte(htmlStr), 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s html file", powerLogFileName)
	}

	return nil
}

// savePowerLogJSON saves the power log as a json file format.
func savePowerLogJSON(ctx context.Context, outDir string, powerLogDict map[string]interface{}, fileSuffix string) error {
	filePath := path.Join(outDir, powerLogFileName+fileSuffix+".json")
	j, err := json.MarshalIndent(powerLogDict, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshall data for %s json file", powerLogFileName)
	}
	if err := os.WriteFile(filePath, j, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s json file", powerLogFileName)
	}

	return nil
}

// uploadToDashboard uploads the power test metrics to go/power-dashboard-view.
func uploadToDashboard(ctx context.Context, powerLogDict map[string]interface{}, uploadurl string) error {
	var urlActual string
	if uploadurl == "" {
		urlActual = "http://chrome-power.appspot.com/rapl"
	} else {
		urlActual = uploadurl
	}
	powerLogJSON, err := json.Marshal(powerLogDict)
	if err != nil {
		return errors.Wrap(err, "failed to marshal data when uploading to dashboard")
	}
	urlParams := url.Values{}
	urlParams.Add("data", string(powerLogJSON))

	const (
		retryAttempts = 9
		exponentBase  = 2
	)
	return action.RetryWithExponentialBackoff(retryAttempts, func(ctx context.Context) error {
		resp, err := http.PostForm(urlActual, urlParams)
		if err != nil {
			return errors.Wrap(err, "failed to upload to power dashboard")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return errors.New("unsuccessful http response from power dashboard: " + resp.Status)
		}

		return nil
	}, time.Second, exponentBase)(ctx)
}

// tagTimelineWithCheckpoints assumes that values come from perf.Timeline. For
// every data point, it generates a list of Checkpoint names that describe this
// data point.
// TODO(b/315282941): while the data are tagged correctly here, power dashboard
// UI shows that checkpoint bar does not perfectly align with data points. This
// may be due to the fact that some data points do not have any tags. Need to
// fix the UI later.
func tagTimelineWithCheckpoints(values *perf.Values, checkpoints *perf.Checkpoints) ([][]string, error) {
	// Return nil if there is no checkpoints.
	if checkpoints == nil || checkpoints.IsEmpty() {
		return nil, nil
	}

	btoi := map[bool]int{true: 1, false: -1}

	// Look for the 1 timestampSource Metric.
	var tsMetric perf.Metric
	foundTsMetric := false
	for metric := range values.GetValues() {
		if metric.HasStartTs {
			tsMetric = metric
			foundTsMetric = true
			break
		}
	}
	if !foundTsMetric {
		return nil, errors.New("values does not contain timeline timestamps")
	}

	// Calculate Unix timestamps from relative timestamps in perf.Timeline.
	unixTss, err := values.ConvertToUnixTss(tsMetric)
	if err != nil {
		return nil, err
	}

	checkpointTags := make([][]string, len(values.GetValueByMetric(tsMetric)))

	// Compare Unix timestamps with Checkpoints. If a Unix timestamp is within
	// some Checkpoints coverage, tag it with the Checkpoint names.
	j := 0
	events := checkpoints.Flatten()
	started := make(map[string]int)
	tagsEmpty := true

	for i, unixTs := range unixTss {
		for ; j < len(events) && !events[j].Ts.After(unixTs); j++ {
			// Count the Checkpoints that are on now for a timestamp.
			name := events[j].CheckpointName
			started[name] += btoi[events[j].IsStart]
			if started[name] == 0 {
				delete(started, name)
			}
		}
		// Tag each Unix timestamp with the Checkpoint names.
		visible := 0
		if len(started) > 0 {
			tagsEmpty = false
			for tag := range started {
				checkpointTags[i] = append(checkpointTags[i], tag)
				if !strings.HasPrefix(tag, "loop") && !strings.HasPrefix(tag, "_") {
					visible++
				}
			}
		}
		// The power dashboard runs into UI misalignment if a data point has
		// no visible checkpoint tags.
		if visible == 0 {
			checkpointTags[i] = append(checkpointTags[i], "none")
		}

	}

	// If there is no valid checkpoint for the entire test, return nil to save
	// upload file size.
	if tagsEmpty {
		return nil, nil
	}

	return checkpointTags, nil
}
