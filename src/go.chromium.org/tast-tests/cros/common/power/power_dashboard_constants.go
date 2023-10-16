// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

// Define a type for each metric. Metric "type" will be added as a prefix before the metric name.
// This is to help categorize each metric in power_log.json/.html, which can also be used as a
// filter on power_dashboard.
const (
	CPUIdleMetricType          = "cpuidle."
	CPUUsageMetricType         = "cpu_usage."
	FanMetricType              = "fan."
	FPSMetricType              = "fps."
	GeneralPerfMetricType      = "perf."
	GPUFreqMetricType          = "gpufreq_wavg."
	GPUUsageMetricType         = "gpu_usage."
	PackageCstatesMetricType   = "cpupkg."
	BatterySOCMetricType       = "battery."
	HistogramMetricType        = "histogram."
	PowerRelatedMetricType     = "power."
	ThermalMetricType          = "temperature."
	WebrtcBitrateMetricType    = "webrtc_bitrate."
	WebrtcFpsMetricType        = "webrtc_fps."
	WebrtcLimitationMetricType = "webrtc_limitation."
	WebrtcPixelMetricType      = "webrtc_pixel."
	WebrtcTimeMetricType       = "webrtc_time."
	WebrtcQPMetricType         = "webrtc_qp."
	ZramMetricType             = "zram."
	MemoryMetricType           = "memory."
)

// Units for each metric type.
const (
	CPUIdleMetricTypeUnit          = "percent"
	CPUUsageMetricTypeUnit         = "percent"
	FanMetricTypeUnit              = "rpm"
	FPSMetricTypeUnit              = "fps"
	GeneralPerfMetricTypeUnit      = "point"
	GPUFreqMetricTypeUnit          = "megahertz"
	GPUUsageUtilizationTypeUnit    = "percent"
	GPUUsageMemoryTypeUnit         = "kiB"
	HistogramLatencyMetricTypeUnit = "us"
	PackageCstatesMetricTypeUnit   = "percent"
	PowerRelatedMetricTypeUnit     = "W"
	ThermalMetricTypeUnit          = "celsius"
	WebrtcBitrateMetricTypeUnit    = "kbps"
	WebrtcFpsMetricTypeUnit        = "fps"
	WebrtcLimitationMetricTypeUnit = "percent"
	WebrtcPixelMetricTypeUnit      = "pixel"
	WebrtcTimeMetricTypeUnit       = "ms"
	WebrtcQPMetricTypeUnit         = "point"
	ZramMetricTypeUnit             = "requests"
	MemoryMetricTypeUnit           = "kiB"
)

// Keys for battery life metrics in power log.
const (
	MinutesBatteryLifeKey       = "minutes_battery_life"
	MinutesBatteryLifeTestedKey = "minutes_battery_life_tested"
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
