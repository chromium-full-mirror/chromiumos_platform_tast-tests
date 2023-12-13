// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metrics contains the Phoronix-test-suite metrics results parser to
// convert the results to crosbolt results-charts.json.
package metrics

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/crospts/ptsworld"
	"go.chromium.org/tast/core/errors"
)

type metricType string

const (
	memoryMetric metricType = "memory"
	cpuMetric    metricType = "processor"
	diskMetric   metricType = "disk"
	systemMetric metricType = "system"
)

// crosboltUnit is the structure of PTS scale mapping to crosbolt unit.
type crosboltUnit struct {
	// The unit of the crosbolt result.
	unit string
	// The suffix of the name in crosbolt results-chart.json.
	nameSuffix string
}

// crosboltUnitMap is a map from PTS scale to crosbolt unit.
type crosboltUnitMap map[string]crosboltUnit

// crosboltUnitTable is the table of PTS scale maps to crosbolt unit and name
// suffix.
var crosboltUnitTable = []crosboltUnitMap{
	{
		"MB/s": {
			unit:       "MB",
			nameSuffix: "MB_Per_Sec",
		},
	},
	{
		"MiB/s": {
			unit:       "MiB",
			nameSuffix: "MiB_Per_Sec",
		},
	},
	{
		"Microseconds Per Op": {
			unit:       "ms",
			nameSuffix: "MS_Per_Op",
		},
	},
	{
		"MIPS": {
			unit:       "MIPS",
			nameSuffix: "Mil_Inst_Per_Sec",
		},
	},
	{
		"byte/s": {
			unit:       "Byte",
			nameSuffix: "Byte_Per_Sec",
		},
	},
	{
		"Megapixels/sec": {
			unit:       "MegaPixels",
			nameSuffix: "MegaPixels_Per_Sec",
		},
	},
	{
		"Seconds": {
			unit:       "s",
			nameSuffix: "Sec",
		},
	},
	{
		"Frames Per Second": {
			unit:       "FPS",
			nameSuffix: "Frame_Per_Sec",
		},
	},
	{
		"Microseconds": {
			unit:       "us",
			nameSuffix: "Micro_Sec",
		},
	},
}

func toCrosboltUnit(scale string) (crosboltUnit, error) {
	for _, unit := range crosboltUnitTable {
		if _, ok := unit[scale]; ok {
			return unit[scale], nil
		}
	}
	return crosboltUnit{}, errors.Errorf("unknown scale %s", scale)
}

func toCrosboltDirection(proportion string) (perf.Direction, error) {
	if proportion == "LIB" {
		return perf.SmallerIsBetter, nil
	}
	if proportion == "HIB" {
		return perf.BiggerIsBetter, nil
	}
	return -1, errors.Errorf("unknown proportion %s", proportion)

}

// The composite.xml is the Phoronix-Test-Suite test result in xml format. The
// metrics in composite.xml are recorded in Result field which we will convert
// the results to crosbolt results-chart.json format.
// The example of composite.xml is below:
//
// <?xml version="1.0" encoding="UTF-8"?>
// <PhoronixTestSuite>
// <Result>
// <Identifier>pts/leveldb-1.0.2</Identifier>
//  <Title>LevelDB</Title>
// <AppVersion>1.22</AppVersion>
// <Arguments>--benchmarks=readhot --num=1000000</Arguments>
// <Description>Benchmark: Hot Read</Description>
// <Scale>Microseconds Per Op</Scale>
// <Proportion>LIB</Proportion>
// <DisplayFormat>BAR_GRAPH</DisplayFormat>
// <Data>
//   <Entry>
//     <Identifier>PersistentDisk</Identifier>
//     <Value>0.000</Value>
//     <RawString>0.000:0.000:0.000</RawString>
//     <JSON>{"compiler-options":{"compiler-type":"CXX","compiler":"g++","compiler-options":"-O3 -lsnappy"},"test-run-times":"3.85:3.75:3.74"}</JSON>
//  </Entry>
// </Data>
// </Result>
// </PhoronixTestSuite>
//
// The composite.xml is located in the test results directory with the format of
// "yyyy-mm-dd-hhmm".

// phoronixTestSuite is the structure of composite.xml.
type phoronixTestSuite struct {
	Results []result `xml:"Result"`
}

type result struct {
	Identifier    string `xml:"Identifier"`
	Title         string `xml:"Title"`
	AppVersion    string `xml:"AppVersion"`
	Description   string `xml:"Description"`
	Scale         string `xml:"Scale"`
	Proportion    string `xml:"Proportion"`
	DisplayFormat string `xml:"DisplayFormat"`
	Data          data   `xml:"Data"`
}

type data struct {
	Entry entry `xml:"Entry"`
}

type entry struct {
	Value float64 `xml:"Value"`
}

// ptsComposite is the structure of PTS composite results to convert to
// crosbolt.
type ptsComposite struct {
	// ConvertToCrosbolt converts the PTS results to crosbolt results.
	crosboltNameTable map[string]string
	// metricType is the type of the metric.
	metricType metricType
}

// ResultsParser parses the PTS results from composite.xml results to crosbolt
// results-chart.json using perf.Values.
type ResultsParser struct {
	resultsDir string
	ptsType    ptsworld.PtsType
	pv         *perf.Values
	//ptsComposite is a map from test name to ptsComposite interface.
	ptsComposite map[string]ptsComposite
}

// getLatestResultDir returns the latest result directory in the given results
// directory. The result directory is named with the format of
// "yyyy-mm-dd-hhmm".
func (r *ResultsParser) getLatestResultDir() (string, error) {
	const resultDirFormat = "2006-01-02-1504"
	var latestDir string
	var latestTime time.Time

	files, err := os.ReadDir(r.resultsDir)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.IsDir() {
			dirTime, err := time.Parse(resultDirFormat, file.Name())
			if err != nil {
				continue
			}
			if dirTime.After(latestTime) {
				latestDir = file.Name()
				latestTime = dirTime
			}
		}
	}
	if latestDir == "" {
		return "", errors.Errorf("no results found in %s", r.resultsDir)
	}
	latestDir = filepath.Join(r.resultsDir, latestDir)

	// Update the symlink of latest result directory to "latest" folder
	symLatestDir := filepath.Join(r.resultsDir, "latest")
	if _, err = os.Lstat(symLatestDir); err == nil {
		err = os.Remove(symLatestDir)
		if err != nil {
			return "", errors.Wrapf(err, "error removing existing symlink: %s", symLatestDir)
		}
	}
	if err = os.Symlink(latestDir, symLatestDir); err != nil {
		if !os.IsExist(err) {
			return "", errors.Wrapf(err, "error creating symlink: %s", symLatestDir)
		}
	}

	return latestDir, nil
}

// NewResultsParser creates a new ResultsParser.
func NewResultsParser(resultsDir string, ptsType ptsworld.PtsType) *ResultsParser {
	return &ResultsParser{
		resultsDir: resultsDir,
		ptsType:    ptsType,
		pv:         perf.NewValues(),
		// TODO(darrenwu): Add more test results
		ptsComposite: map[string]ptsComposite{
			leveldbTitle: {
				crosboltNameTable: leveldbCrosboltNameTable,
				metricType:        systemMetric,
			},
			mbwTitle: {
				crosboltNameTable: mbwCrosboltNameTable,
				metricType:        memoryMetric,
			},
			cachebenchTitle: {
				crosboltNameTable: cachebenchCrosboltNameTable,
				metricType:        cpuMetric,
			},
			compress7zipTitle: {
				crosboltNameTable: compress7zipCrosboltNameTable,
				metricType:        cpuMetric,
			},
			opensslTitle: {
				crosboltNameTable: opensslCrosboltNameTable,
				metricType:        cpuMetric,
			},
			tjbenchTitle: {
				crosboltNameTable: tjbenchCrosboltNameTable,
				metricType:        systemMetric,
			},
			compresslz4Title: {
				crosboltNameTable: compresslz4CrosboltNameTable,
				metricType:        cpuMetric,
			},
			encodeMp3Title: {
				crosboltNameTable: encodeMp3CrosboltNameTable,
				metricType:        systemMetric,
			},
			vpxencTitle: {
				crosboltNameTable: vpxencCrosboltNameTable,
				metricType:        cpuMetric,
			},
			tensorflowLiteTitle: {
				crosboltNameTable: tensorflowLiteCrosboltNameTable,
				metricType:        systemMetric,
			},
			rnNoiseTitle: {
				crosboltNameTable: rnNoiseCrosboltNameTable,
				metricType:        cpuMetric,
			},
			cythonBenchTitle: {
				crosboltNameTable: cythonBenchCrosboltNameTable,
				metricType:        cpuMetric,
			},
		},
	}
}

// convertToCrosbolt converts the PTS results to crosbolt results.
func convertToCrosbolt(crosboltNameTable map[string]string, metricType metricType, result *result, ptsType ptsworld.PtsType, pv *perf.Values) error {

	if name, ok := crosboltNameTable[result.Description]; ok {
		crosboltUnit, err := toCrosboltUnit(result.Scale)
		if err != nil {
			return err
		}
		direction, err := toCrosboltDirection(result.Proportion)
		if err != nil {
			return err
		}
		// Remove the space in title to conform the name format
		title := strings.Replace(result.Title, " ", "", -1)
		pv.Set(perf.Metric{
			Name:      fmt.Sprintf("%v.%v.%v.%v_%v", ptsType, metricType, title, name, crosboltUnit.nameSuffix),
			Unit:      crosboltUnit.unit,
			Direction: direction,
		}, result.Data.Entry.Value)
	}

	return nil
}

// ConvertMetrics converts the PTS results from composite.xml in the latest
// result directory to crosbolt results-chart.json using perf.Values.
func (r *ResultsParser) ConvertMetrics(outDir string) error {
	latestResultDir, err := r.getLatestResultDir()
	if latestResultDir == "" || err != nil {
		return errors.New("failed to get latest result directory")
	}

	compositeData, err := os.ReadFile(filepath.Join(latestResultDir, "composite.xml"))
	if err != nil {
		return errors.Wrap(err, "failed to read composite.xml")
	}

	var ptsXML phoronixTestSuite
	err = xml.Unmarshal(compositeData, &ptsXML)
	if err != nil {
		return errors.Wrap(err, "failed to unmarshal composite.xml")
	}

	for _, result := range ptsXML.Results {
		if p, ok := r.ptsComposite[result.Title]; ok {
			err = convertToCrosbolt(p.crosboltNameTable, p.metricType, &result, r.ptsType, r.pv)
			if err != nil {
				return errors.Wrapf(err, "failed to convert phoronix-test-suite %s results to crosbolt results-chart.json", result.Title)
			}
		}
	}
	r.pv.Save(outDir)
	return nil
}
