// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"encoding/csv"
	"log"
	"math"
	"os"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

type hmrNode struct {
	coorX    float64
	coorY    float64
	pressure float64
	time     float64
}

// distance calculates the euclidean distance between two hmrNode points.
func distance(pointA, pointB hmrNode) float64 {
	return math.Sqrt(math.Pow(pointA.coorX-pointB.coorX, 2) + math.Pow(pointA.coorY-pointB.coorY, 2))
}

// readCSV reads a csv file with columns x, y, pressure and time.
// Each line in the csv ends with a ','.
func readCSV(fileName string) ([]*hmrNode, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "attempt to open csv file (%s) failed", fileName)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Dataset lines end with ',', whereas the header line doesn't. -1 to compensate.
	// TODO: b/304852953 - Transition to another output format that contains the same number of columns in the header & dataset.
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		errors.Wrap(err, "attempt to read csv failed")
	}

	columnToIdx := map[string]int{}
	for i, name := range records[0] {
		columnToIdx[name] = i
	}

	records = records[1:]
	var points []*hmrNode
	for _, row := range records {
		points = append(points, &hmrNode{stringToFloat64(row[columnToIdx["x"]]), stringToFloat64(row[columnToIdx["y"]]), stringToFloat64(row[columnToIdx["pressure"]]), stringToFloat64(row[columnToIdx["time"]])})
	}
	return points, nil
}

func stringToFloat64(str string) float64 {
	if str == "" {
		str = "0.0"
	}

	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		log.Fatal(err)
	}
	return f
}

// removeStationaryPoints removes all points at the start of an array of hmrNodes that are within a radius of the starting point.
func removeStationaryPoints(points []*hmrNode, radius float64) []*hmrNode {
	if radius == 0 || len(points) == 0 {
		return points
	}
	startingX := points[0].coorX
	startingY := points[0].coorY
	var offsetX, offsetY float64

	for i, point := range points {
		offsetX = startingX - point.coorX
		offsetY = startingY - point.coorY

		distance := math.Sqrt(math.Pow(offsetX, 2) + math.Pow(offsetY, 2))
		if distance > radius {
			return points[i:]
		}
	}

	return make([]*hmrNode, 0)
}

// removeInitialTouch removes all points that occurred before the most recent stylus hover. Only hovers that started within radius distance of the starting point are checked.
func removeInitialTouch(points []*hmrNode, radius float64) []*hmrNode {
	if radius == 0 || len(points) == 0 {
		return points
	}
	startingX := points[0].coorX
	startingY := points[0].coorY
	var offsetX, offsetY float64

	lastHoverIdx := -1

	for i, point := range points {
		// If the stylus is currently hovering, we will continue to remove points until the stylus touches again. Even if those points are outside the radius.
		if point.pressure == 0 && lastHoverIdx+1 == i {
			lastHoverIdx = i
			continue
		}

		offsetX = startingX - point.coorX
		offsetY = startingY - point.coorY
		distance := math.Sqrt(math.Pow(offsetX, 2) + math.Pow(offsetY, 2))
		if distance > radius {
			break
		}

		if point.pressure == 0 {
			lastHoverIdx = i
		}
	}
	return points[lastHoverIdx+1:]
}
