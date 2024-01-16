// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

import (
	"math"
	"sort"
)

type numeric interface {
	uint32 | float64
}

// DistanceDataStats contains relevant statistical data for distance in a ranging session
type DistanceDataStats struct {
	Mean                     float64
	Median                   float64
	StandardDeviation        float64
	WithinBoundaryPercentage float64
}

// AngleDataStats contains relevant statistical data for angles in a ranging session
type AngleDataStats struct {
	Mean                     float64
	Median                   float64
	StandardDeviation        float64
	WithinBoundaryPercentage float64
	AboveMinFOMPercentage    float64
}

// RangingSessionSummary contains relevant statistical data for a ranging session
type RangingSessionSummary struct {
	TotalReportCount          uint32
	SuccessfulReportCount     uint32
	SuccessReportRate         float64
	StatusCodes               map[int32]uint64
	DistanceStats             DistanceDataStats
	AzimuthStats              AngleDataStats
	DestinationAzimuthStats   AngleDataStats
	ElevationStats            AngleDataStats
	DestinationElevationStats AngleDataStats
}

// ExpectedRangingBounds contains the bounds that denote the acceptable distances and angles
// These bounds should be set based off of the devices' actual physical positions
type ExpectedRangingBounds struct {
	DistanceUpperBound  uint32
	DistanceLowerBound  uint32
	AzimuthUpperBound   float64
	AzimuthLowerBound   float64
	ElevationUpperBound float64
	ElevationLowerBound float64
	MinimumFOM          uint32
}

// GetRangingSessionSummary takes a RangingResult and based off the ExpectedRangingBounds, returns a summary of the relevant statistical calculations
// Azimuth and Elevation should be set to true if those are to be evaluated, false otherwise
func GetRangingSessionSummary(result *RangingResult, expected *ExpectedRangingBounds, azimuth, elevation bool) (*RangingSessionSummary, error) {
	report := &RangingSessionSummary{
		TotalReportCount:      result.TotalReportCount,
		SuccessfulReportCount: result.SuccessfulReportCount,
		SuccessReportRate:     (float64(result.SuccessfulReportCount) / float64(result.TotalReportCount)) * 100,
		StatusCodes:           result.StatusCodes,
		DistanceStats:         getDistanceDataStats(result.DistanceReadings, expected.DistanceUpperBound, expected.DistanceLowerBound),
	}

	if azimuth {
		report.AzimuthStats = getAngleDataStats(result.AzimuthReadings, expected.AzimuthUpperBound, expected.AzimuthLowerBound, expected.MinimumFOM)
	}

	if elevation {
		report.ElevationStats = getAngleDataStats(result.ElevationReadings, expected.ElevationUpperBound, expected.ElevationLowerBound, expected.MinimumFOM)
	}
	return report, nil
}

// getDistanceDataStats generates the relevant statistical data from a set of data and given bounds
func getDistanceDataStats(data []uint32, upperBound, lowerBound uint32) DistanceDataStats {
	mean := mean(data)

	if len(data) == 0 {
		return DistanceDataStats{}
	}

	return DistanceDataStats{
		Mean:                     mean,
		Median:                   median(data),
		StandardDeviation:        standardDeviation(data, mean),
		WithinBoundaryPercentage: withinBoundaryPercentage(data, upperBound, lowerBound),
	}

}

// getAngleDataStats generates the relevant statistical data from a set of data and given bounds
func getAngleDataStats(data []*AngleReading, upperBound, lowerBound float64, minFOM uint32) AngleDataStats {
	stats := AngleDataStats{}

	if len(data) == 0 {
		return stats
	}

	anglesOnly := getAoaOnly(data)
	mean := mean(anglesOnly)
	boundary, FOM := withinBoundaryPercentageAndFOM(data, upperBound, lowerBound, minFOM)

	return AngleDataStats{
		Mean:                     mean,
		Median:                   median(anglesOnly),
		StandardDeviation:        standardDeviation(anglesOnly, mean),
		WithinBoundaryPercentage: boundary,
		AboveMinFOMPercentage:    FOM,
	}
}

func mean[T numeric](data []T) float64 {
	var total float64
	for _, value := range data {
		total += float64(value)
	}
	return total / float64(len(data))
}

func median[T numeric](data []T) float64 {
	if len(data) == 1 {
		return float64(data[0])
	}

	// Sort the slice
	sort.Slice(data, func(i, j int) bool {
		return data[i] < data[j]
	})

	middle := len(data) / 2

	// Calculate the median based on whether the length is even or odd
	if len(data)%2 == 0 {
		return (float64(data[middle-1]) + float64(data[middle])) / 2.0
	}
	return float64(data[middle])
}

func standardDeviation[T numeric](data []T, mean float64) float64 {
	var sd float64

	for _, value := range data {
		sd += math.Pow(float64(value)-mean, 2)
	}

	return math.Sqrt(sd / float64(len(data)))
}

// withinBoundaryPercentage accepts a set of data, upper and lower bound (inclusive). It returns
// percentage of data within the bounds as a float
func withinBoundaryPercentage(data []uint32, upperBound, lowerBound uint32) float64 {
	total := len(data)
	if total == 0 {
		return 0
	}
	validCount := 0
	for _, value := range data {
		if value >= lowerBound && value <= upperBound {
			validCount++
		}
	}
	return (float64(validCount) / float64(total)) * 100
}

// withinBoundaryPercentageAndFOM accepts a set of data, upper and lower bound (inclusive). It returns
// percentage of data within the bounds as a float
func withinBoundaryPercentageAndFOM(data []*AngleReading, upperBound, lowerBound float64, minFOM uint32) (float64, float64) {
	total := len(data)
	if total == 0 {
		return 0, 0
	}
	validCount := 0
	validFOMCount := 0
	for _, value := range data {
		if value.Reading >= lowerBound && value.Reading <= upperBound {
			validCount++
		}
		if value.Fom >= minFOM {
			validFOMCount++
		}

	}
	return (float64(validCount) / float64(total)) * 100, (float64(validFOMCount) / float64(total)) * 100
}

func getAoaOnly(data []*AngleReading) []float64 {
	var aoa []float64
	for _, value := range data {
		aoa = append(aoa, value.Reading)
	}
	return aoa
}
