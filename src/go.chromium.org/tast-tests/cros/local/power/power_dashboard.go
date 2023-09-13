// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/power/util"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Define a type for each metric. Metric "type" will be added as a prefix before the metric name.
// This is to help categorize each metric in power_log.json/.html, which can also be used as a
// filter on power_dashboard.
const (
	cpuIdleMetricType          = "cpuidle."
	cpuUsageMetricType         = "cpu_usage."
	fanMetricType              = "fan."
	fpsMetricType              = "fps."
	generalPerfMetricType      = "perf."
	gpuFreqMetricType          = "gpufreq_wavg."
	gpuUsageMetricType         = "gpu_usage."
	packageCstatesMetricType   = "cpupkg."
	batterySOCMetricType       = "battery."
	histogramMetricType        = "histogram."
	powerRelatedMetricType     = "power."
	thermalMetricType          = "temperature."
	webrtcBitrateMetricType    = "webrtc_bitrate."
	webrtcFpsMetricType        = "webrtc_fps."
	webrtcLimitationMetricType = "webrtc_limitation."
	webrtcPixelMetricType      = "webrtc_pixel."
	webrtcTimeMetricType       = "webrtc_time."
	webrtcQPMetricType         = "webrtc_qp."
	zramMetricType             = "zram."
	memoryMetricType           = "memory."
)

// Only keys inside validMetricTypeMap are accepted metric types.
var validMetricTypeMap = map[string]bool{
	"battery":           true,
	"cpuidle":           true,
	"cpu_usage":         true,
	"fan":               true,
	"fps":               true,
	"perf":              true,
	"gpufreq_wavg":      true,
	"gpu_usage":         true,
	"cpupkg":            true,
	"histogram":         true,
	"power":             true,
	"temperature":       true,
	"webrtc_bitrate":    true,
	"webrtc_fps":        true,
	"webrtc_pixel":      true,
	"webrtc_limitation": true,
	"webrtc_time":       true,
	"webrtc_qp":         true,
	"zram":              true,
	"memory":            true,
	"other":             true,
}

// Units for each metric type.
const (
	cpuIdleMetricTypeUnit          = "percent"
	cpuUsageMetricTypeUnit         = "percent"
	fanMetricTypeUnit              = "rpm"
	fpsMetricTypeUnit              = "fps"
	generalPerfMetricTypeUnit      = "point"
	gpuFreqMetricTypeUnit          = "megahertz"
	gpuUsageUtilizationTypeUnit    = "percent"
	gpuUsageMemoryTypeUnit         = "kiB"
	histogramLatencyMetricTypeUnit = "us"
	packageCstatesMetricTypeUnit   = "percent"
	powerRelatedMetricTypeUnit     = "W"
	thermalMetricTypeUnit          = "celsius"
	webrtcBitrateMetricTypeUnit    = "kbps"
	webrtcFpsMetricTypeUnit        = "fps"
	webrtcLimitationMetricTypeUnit = "percent"
	webrtcPixelMetricTypeUnit      = "pixel"
	webrtcTimeMetricTypeUnit       = "ms"
	webrtcQPMetricTypeUnit         = "point"
	zramMetricTypeUnit             = "requests"
	memoryMetricTypeUnit           = "kiB"
)

// Power log file name.
const powerLogFileName = "power_log"

const htmlChartStr = `
<!DOCTYPE html>
<html>
<head>
<script type="text/javascript" src="https://www.gstatic.com/charts/loader.js">
</script>
<script type="text/javascript">
    google.charts.load('current', {'packages':['corechart', 'table']});
    google.charts.setOnLoadCallback(drawChart);
    function drawChart() {
        var dataArray = [
{data}
        ];
        var data = google.visualization.arrayToDataTable(dataArray);
        var numDataCols = data.getNumberOfColumns() - 1;
        var unit = '{unit}';
        var type = '{type}';
        var options = {
            width: 1600,
            height: 1200,
            lineWidth: 1,
            legend: { position: 'top', maxLines: 3 },
            vAxis: {viewWindow: {min: 0}, title: '{type} ({unit})'},
            hAxis: {viewWindow: {min: 0}, title: 'time (second)'},
        };
        var element = document.getElementById('{type}');
        var chart;
        if (unit == 'percent' && numDataCols >= 2) {
            options['isStacked'] = true;
            if (numDataCols == 2) {
                options['colors'] = ['#d32f2f', '#43a047']
            } else if (numDataCols <= 4) {
                options['colors'] = ['#d32f2f', '#f4c7c3', '#cddc39','#43a047'];
            } else if (numDataCols <= 9) {
                options['colors'] = ['#d32f2f', '#e57373', '#f4c7c3', '#ffccbc',
                        '#f0f4c3', '#c8e6c9', '#cddc39', '#81c784', '#43a047'];
            }
            chart = new google.visualization.SteppedAreaChart(element);
        } else if (data.getNumberOfRows() == 1 && type == 'perf') {
            var newArray = [['key', 'value']];
            for (var i = 1; i < dataArray[0].length; i++) {
                newArray.push([dataArray[0][i], dataArray[1][i]]);
            }
            data = google.visualization.arrayToDataTable(newArray);
            delete options.width;
            delete options.height;
            chart = new google.visualization.Table(element);
        } else {
            chart = new google.visualization.LineChart(element);
        }
        chart.draw(data, options);
    }
</script>
</head>
<body>
<div id="{type}"></div>
</body>
</html>
`

const (
	minutesBatteryLifeKey       = "minutes_battery_life"
	minutesBatteryLifeTestedKey = "minutes_battery_life_tested"
)

// ConvertPowerPerfValue converts raw performance metric values to power dictionary.
func ConvertPowerPerfValue(ctx context.Context, values *perf.Values) (map[string]interface{}, error) {
	measurement := values.GetValues()
	if measurement == nil || len(measurement) == 0 {
		return nil, errors.New("invalid power measurement")
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
			continue
		} else if size == 1 {
			metricType = "other"
		} else {
			metricType = metricNameSlice[0]
			metricName = strings.ToLower(strings.Join(metricNameSlice[1:size], "_"))
		}

		// Validate metricType.
		if _, ok := validMetricTypeMap[metricType]; !ok {
			return nil, errors.Errorf("unexpected metric type %q for %q", metricType, metricName)
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
	if value, ok := innerAverageMap[minutesBatteryLifeTestedKey]; ok {
		totalDurationSec = value * 60
	}
	if value, ok := innerDataMap["t"]; ok {
		var sampleCount = len(value)
		powerDict["sample_count"] = sampleCount
		if sampleCount > 0 {
			lastTimestamp := value[sampleCount-1]
			powerDict["sample_duration"] = lastTimestamp / float64(sampleCount)
		}
	}

	minutesBatteryLife := getMinutesBatteryLife(ctx, innerDataMap, innerAverageMap, totalDurationSec)
	values.Set(perf.Metric{
		Name:      generalPerfMetricType + minutesBatteryLifeKey,
		Unit:      "minute",
		Direction: perf.BiggerIsBetter,
	}, minutesBatteryLife)
	innerDataMap[minutesBatteryLifeKey] = []float64{minutesBatteryLife}
	innerAverageMap[minutesBatteryLifeKey] = minutesBatteryLife
	typeMap[minutesBatteryLifeKey] = "perf"
	unitMap[minutesBatteryLifeKey] = "minute"

	typeMap[minutesBatteryLifeTestedKey] = "perf"

	// Check if package-0 is collected first because `rapl` is not supported on all platforms.
	if _, ok := innerDataMap[package0]; ok && len(innerDataMap["system"]) == len(innerDataMap[package0]) {
		innerDataMap["non_SoC"], innerAverageMap["non_SoC"] = getNonSocSubsystemPowerData(ctx, innerDataMap, innerAverageMap)
		typeMap["non_SoC"] = "power"
		unitMap["non_SoC"] = powerRelatedMetricTypeUnit
		values.Append(perf.Metric{
			Name:     powerRelatedMetricType + "non_SoC",
			Unit:     powerRelatedMetricTypeUnit,
			Multiple: true,
		}, innerDataMap["non_SoC"]...)
	}

	updatePowerLogPerf(ctx, innerDataMap, innerAverageMap, typeMap, unitMap)

	powerDict["data"] = innerDataMap
	powerDict["average"] = innerAverageMap
	powerDict["type"] = typeMap
	powerDict["unit"] = unitMap
	return powerDict, nil
}

// getMinutesBatteryLife calculates and returns the projected operating minutes.
func getMinutesBatteryLife(ctx context.Context, innerDataMap map[string][]float64, innerAverageMap map[string]float64, totalDurationSec float64) (minutesBatteryLife float64) {
	// Power key value calculation.
	batteryPath, err := SysfsBatteryPath(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to calculate key value: ", err)
		return 0
	}

	chargeFullDesign, err := ReadBatteryChargeDesignSize(ctx, batteryPath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get battery charge design size: ", err)
		return 0
	}

	chargeFull, err := ReadBatteryChargeSize(ctx, batteryPath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get battery charge size: ", err)
		return 0
	}

	energyFullDesign, err := ReadBatteryDesignEnergySize(ctx, batteryPath)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get battery design energy size: ", err)
		return 0
	}

	if energyUsed, ok := innerAverageMap["discharge_mwh"]; ok && energyUsed > 0 && totalDurationSec > 0 {
		lowBatteryShutdownPercent, err := LowBatteryShutdownPercent(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Failed to read low battery shut down percent: Use 4% for approximation")
			lowBatteryShutdownPercent = 4.0
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
	SocPowerNumbers := innerDataMap[package0]
	// All subsystem(nonSoc) power data.
	nonSoCPowerNumbers := make([]float64, 0)

	for index := 0; index < len(systemPowerNumbers); index++ {
		nonSoCPowerNumbers = append(nonSoCPowerNumbers, systemPowerNumbers[index]-SocPowerNumbers[index])
	}

	nonSoCPowerAverage := innerAverageMap["system"] - innerAverageMap[package0]
	return nonSoCPowerNumbers, nonSoCPowerAverage
}

// updatePowerLogPerf adds perf scalar to power log map.
func updatePowerLogPerf(ctx context.Context, dataMap map[string][]float64, averageMap map[string]float64, typeMap, unitMap map[string]string) {
	// Backlight scalars.
	const (
		nonlinearKey = "level_backlight_percent_nonlinear"
		linearKey    = "level_backlight_percent_linear"
	)
	nonlinear, linear := util.GetBacklightLevel(ctx)

	dataMap[nonlinearKey] = []float64{nonlinear}
	averageMap[nonlinearKey] = nonlinear
	typeMap[nonlinearKey] = "perf"
	unitMap[nonlinearKey] = generalPerfMetricTypeUnit

	dataMap[linearKey] = []float64{linear}
	averageMap[linearKey] = linear
	typeMap[linearKey] = "perf"
	unitMap[linearKey] = generalPerfMetricTypeUnit
}

// CreatePowerLogDict creates the power log dictionary from power dict.
func CreatePowerLogDict(ctx context.Context, testName string, powerDict map[string]interface{}, args ...OptionalRecorderArg) map[string]interface{} {
	powerLogDict := map[string]interface{}{
		"format_version": 7,
		// TODO: see b/271917877
		// This is the start time of the test
		// Unfortunately, not every tracker records time
		// We can probably ignore this for now
		// We also need to add the following entry to
		// 'google3/experimental/chromeos_power/dashboard/bigquery_schema.json':
		// {
		// 	"description": "Unix timestamp when the test start running.",
		// 	"mode": "NULLABLE",
		// 	"name": "timestamp",
		// 	"type": "TIMESTAMP"
		// 	},
		"timestamp": time.Now().Unix(),
		"test":      testName,
		"dut":       FormatDeviceInfoForPowerLog(GetDeviceInfo(ctx, args...)),
		"power":     powerDict,
	}

	return powerLogDict
}

// SavePowerLogJSON saves the power log as a json file format.
func SavePowerLogJSON(ctx context.Context, outDir string, powerLogDict map[string]interface{}) error {
	filePath := path.Join(outDir, powerLogFileName+".json")
	j, err := json.MarshalIndent(powerLogDict, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshall data for %s json file", powerLogFileName)
	}
	if err := ioutil.WriteFile(filePath, j, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s json file", powerLogFileName)
	}

	return nil
}

// containEmpty is the helper function to check if there is an empty string among args.
func containEmpty(strs ...string) bool {
	for _, str := range strs {
		if str == "" {
			return true
		}
	}
	return false
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

// UploadToDashboard uploads the power test metrics to go/power-dashboard-view.
func UploadToDashboard(ctx context.Context, powerLogDict map[string]interface{}, uploadurl string) error {
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

// SavePowerLogHTML saves the power log as a json file format.
func SavePowerLogHTML(ctx context.Context, outDir string, powerLogDict map[string]interface{}) error {
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
		if strings.Contains(metric, "package-non-C0_C1") {
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

	filePath := path.Join(outDir, powerLogFileName+".html")

	if err := ioutil.WriteFile(filePath, []byte(htmlStr), 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s html file", powerLogFileName)
	}

	return nil
}

// GeneratePowerLog returns the power dict and the power log dict, and
// stores power_log.json and power_log.html.
func GeneratePowerLog(ctx context.Context, outDir, testName string, values *perf.Values, args ...OptionalRecorderArg) (map[string]interface{}, map[string]interface{}, error) {
	powerDict, err := ConvertPowerPerfValue(ctx, values)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to convert power perf values to power dictionary")
	}
	powerLogDict := CreatePowerLogDict(ctx, testName, powerDict, args...)

	if err := SavePowerLogJSON(ctx, outDir, powerLogDict); err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate power_log.json")
	}

	if err := SavePowerLogHTML(ctx, outDir, powerLogDict); err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate power_log.html")
	}
	return powerDict, powerLogDict, nil
}

// GeneratePowerLogAndSaveToCrosbolt generates power_log.{json, html}
// and upload results to Crosbolt.
func GeneratePowerLogAndSaveToCrosbolt(ctx context.Context, outDir, testName string, values *perf.Values, args ...OptionalRecorderArg) error {
	powerDict, powerLogDict, err := GeneratePowerLog(ctx, outDir, testName, values, args...)
	if err != nil {
		return errors.Wrap(err, "failed to generate power log")
	}

	if powerDict == nil {
		testing.ContextLog(ctx, "Power dictionary is empty. Don't save perf values for crosbolt")
		return nil
	}

	if err := values.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data for crosbolt")
	}

	if err := UploadToDashboard(ctx, powerLogDict, ""); err != nil {
		return errors.Wrap(err, "failed to upload to power dashboard")
	}

	return nil
}
