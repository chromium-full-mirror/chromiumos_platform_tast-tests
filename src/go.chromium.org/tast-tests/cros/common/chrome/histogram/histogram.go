// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package histogram

import (
	"fmt"
	"math"
	"strings"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram/histogrampb"
	"go.chromium.org/tast/core/errors"
)

// Histogram contains data from a single Chrome histogram.
type Histogram struct {
	// Name of the histogram.
	Name string
	// The sum of the all entries in the histogram.
	Sum int64 `json:"sum"`
	// Buckets contains ranges of reported values.
	// The buckets are disjoint and stored in ascending order.
	Buckets []Bucket `json:"buckets"`
}

// Validate checks h's buckets and returns an error if any have invalid ranges or overlap.
func (h *Histogram) Validate() error {
	var lastMax int64 = math.MinInt64
	for _, b := range h.Buckets {
		if b.Min >= b.Max {
			return errors.Errorf("invalid bucket [%d,%d)", b.Min, b.Max)
		}
		if b.Min < lastMax {
			return errors.Errorf("bucket [%d,%d) overlaps previous bucket", b.Min, b.Max)
		}
		lastMax = b.Max
	}
	return nil
}

// TotalCount returns the total number of samples stored across all buckets.
func (h *Histogram) TotalCount() int64 {
	var t int64
	for _, b := range h.Buckets {
		t += b.Count
	}
	return t
}

// IsEmpty returns true if this histogram has no data.
func (h *Histogram) IsEmpty() bool {
	return h.TotalCount() == 0
}

// Diff returns a histogram containing the additional samples in h that aren't in old, an older version of the same histogram.
// Buckets that haven't changed are omitted from the returned histogram.
// old must be an earlier snapshot -- an error is returned if any counts decreased or if old contains buckets not present in h.
func (h *Histogram) Diff(old *Histogram) (*Histogram, error) {
	if h.Name != old.Name {
		return nil, errors.Errorf("unmatched histogram, %s vs %s", h.Name, old.Name)
	}
	if len(old.Buckets) > len(h.Buckets) {
		return nil, errors.Errorf("old histogram has %d bucket(s), new only has %d", len(old.Buckets), len(h.Buckets))
	}

	diff := &Histogram{Name: h.Name, Sum: h.Sum - old.Sum}
	oi := 0
	for _, hb := range h.Buckets {
		// If we've already looked at all of the old buckets, copy the new bucket over.
		if oi >= len(old.Buckets) {
			diff.Buckets = append(diff.Buckets, hb)
			continue
		}

		ob := old.Buckets[oi]

		switch {
		case ob.Min < hb.Min:
			// The old histogram shouldn't contain any buckets that aren't in the new one.
			return nil, errors.Errorf("bucket [%d,%d) is present in old histogram but not new one", ob.Min, ob.Max)
		case ob.Min > hb.Min:
			// If this bucket isn't present in the old histogram, just copy it over.
			if ob.Min < hb.Max {
				return nil, errors.Errorf("old bucket [%d,%d) overlaps new bucket [%d,%d)", ob.Min, ob.Max, hb.Min, hb.Max)
			}
			diff.Buckets = append(diff.Buckets, hb)
		case ob.Min == hb.Min:
			// If we're looking at the same bucket in both histograms, save the difference (if any) and move to the next old bucket.
			if ob.Max != hb.Max {
				return nil, errors.Errorf("old bucket [%d,%d) doesn't match new bucket [%d,%d)", ob.Min, ob.Max, hb.Min, hb.Max)
			}
			if hb.Count < ob.Count {
				return nil, errors.Errorf("old bucket [%d,%d) has count %d, new only has %d", ob.Min, ob.Max, ob.Count, hb.Count)
			} else if hb.Count > ob.Count {
				diff.Buckets = append(diff.Buckets, Bucket{hb.Min, hb.Max, hb.Count - ob.Count})
			}
			oi++
		}
	}
	return diff, nil
}

// String contains a human-readable representation of h as "Name: [[0,5):2 [5,10):1 ...]",
// where each space-separated term is "[<min>,<max>):<count>".
func (h *Histogram) String() string {
	var strs []string
	for _, b := range h.Buckets {
		strs = append(strs, fmt.Sprintf("[%d,%d):%d", b.Min, b.Max, b.Count))
	}
	return h.Name + ": [" + strings.Join(strs, " ") + "]; " + fmt.Sprintf("sum %d", h.Sum)
}

// Mean calculates the estimated mean of the histogram values. It is an error
// when there are no data points.
func (h *Histogram) Mean() (float64, error) {
	if h.TotalCount() == 0 {
		return 0, errors.New("no histogram data")
	}
	return float64(h.Sum) / float64(h.TotalCount()), nil
}

// Percentile calculates a given percentile of the histogram. It is an error when there are no data points. The percentile is approximate.
func (h *Histogram) Percentile(percentile int) (float64, error) {
	totalCount := h.TotalCount()
	if totalCount == 0 {
		return 0, errors.New("no histogram data")
	}

	if percentile < 0 {
		return 0, errors.New("percentile is less than 0")
	}

	if percentile > 100 {
		return 0, errors.New("percentile is greater than 100")
	}

	var count int64 = 0
	for _, b := range h.Buckets {
		count += b.Count
		if (count*100)/totalCount >= int64(percentile) {
			return 0.5 * float64(b.Max+b.Min), nil
		}
	}
	return 0, errors.New("buckets contained fewer than total count elements")
}

// Proto converts this Histogram to something that can be passed in a gRPC call.
func (h *Histogram) Proto() *histogrampb.Histogram {
	result := &histogrampb.Histogram{
		Name: h.Name,
		Sum:  h.Sum,
	}
	for _, b := range h.Buckets {
		bp := &histogrampb.Bucket{
			Min:   b.Min,
			Max:   b.Max,
			Count: b.Count,
		}
		result.Buckets = append(result.Buckets, bp)
	}
	return result
}

// Bucket contains a set of reported samples within a fixed range.
type Bucket struct {
	// Min contains the minimum value that can be stored in this bucket.
	Min int64 `json:"min"`
	// Max contains the exclusive maximum value for this bucket.
	Max int64 `json:"max"`
	// Count contains the number of samples that are stored in this bucket.
	Count int64 `json:"count"`
}

// NewHistogramFromProto converts a given protobuf Histogram to Histogram
func NewHistogramFromProto(hpb *histogrampb.Histogram) *Histogram {
	h := &Histogram{
		Name: hpb.Name,
		Sum:  hpb.Sum,
	}
	for _, bp := range hpb.Buckets {
		b := Bucket{
			Min:   bp.Min,
			Max:   bp.Max,
			Count: bp.Count,
		}
		h.Buckets = append(h.Buckets, b)
	}
	return h
}

// DiffHistograms is a convenience function to diff multiple histograms.
func DiffHistograms(older, newer []*Histogram) ([]*Histogram, error) {
	if len(older) != len(newer) {
		return nil, errors.New("histogram count mismatched")
	}

	var result []*Histogram

	for i := 0; i < len(older); i++ {
		if newer[i].Name != older[i].Name {
			return nil, errors.Errorf("unmatched histogram, index %d, %s vs %s", i, newer[i].Name, older[i].Name)
		}
		histogram, err := newer[i].Diff(older[i])
		if err != nil {
			return nil, err
		}
		result = append(result, histogram)
	}
	return result, nil
}
