// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"encoding/xml"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

// Hierarchy dumping android ui xml hierarchy.
type Hierarchy struct {
	XMLName  xml.Name `xml:"hierarchy"`
	Rotation string   `xml:"rotation,attr"`
	Node     Node     `xml:"node"`
}

// Node ui hierarchy node.
type Node struct {
	XMLName       xml.Name `xml:"node"`
	Index         string   `xml:"index,attr"`
	Text          string   `xml:"text,attr"`
	ResourceID    string   `xml:"resource-id,attr"`
	Class         string   `xml:"class,attr"`
	Package       string   `xml:"package,attr"`
	ContentDesc   string   `xml:"content-desc,attr"`
	Checkable     string   `xml:"checkable,attr"`
	Checked       string   `xml:"checked,attr"`
	Clickable     string   `xml:"clickable,attr"`
	Enabled       string   `xml:"enabled,attr"`
	Focusable     string   `xml:"focusable,attr"`
	Focused       string   `xml:"focused,attr"`
	Scrollable    string   `xml:"scrollable,attr"`
	LongClickable string   `xml:"long-clickable,attr"`
	Password      string   `xml:"password,attr"`
	Selected      string   `xml:"selected,attr"`
	Bounds        string   `xml:"bounds,attr"`
	Nodes         []Node   `xml:"node"`
}

// FindElementMode indicates what .
type FindElementMode int

const (
	// FindElementByText indicates use text find the element.
	FindElementByText FindElementMode = iota
	// FindElementByClass indicates use class find the element.
	FindElementByClass
)

// ClickElement clicks android element by text or class node.
func ClickElement(ctx context.Context, dut *dut.DUT, target string, targetMode FindElementMode) error {
	if err := testing.Poll(ctx, func(context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "adb", "exec-out", "uiautomator", "dump", "/dev/tty").Output(exec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to dump ui tree")
		}
		// Declare a struct to unmarshal the XML string
		var hierarchy Hierarchy

		// Unmarshal the XML string
		if err := xml.Unmarshal([]byte(string(out)), &hierarchy); err != nil {
			return errors.Wrap(err, "failed to unmarshal out")
		}
		var foundNode *Node
		if targetMode == FindElementByText {
			foundNode = findNodeByText(hierarchy.Node, target)
		} else if targetMode == FindElementByClass {
			foundNode = findNodeByClass(hierarchy.Node, target)
		}

		if foundNode != nil {
			centerX, centerY, err := parseBounds(foundNode.Bounds)
			if err != nil {
				return errors.Wrap(err, "failed to parse node bounds to center positions")
			}
			const formatBase = 10
			if err := dut.Conn().CommandContext(ctx, "adb", "shell", "input", "tap", strconv.FormatInt(int64(centerX), formatBase), strconv.FormatInt(int64(centerY), formatBase)).Run(exec.DumpLogOnError); err != nil {
				return errors.Wrapf(err, "failed to tap %s", target)
			}
			return nil
		}
		if err := swipeDown(ctx, dut); err != nil {
			return errors.Wrap(err, "failed to swipe down")
		}
		return errors.New("failed to find element")
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to click element")
	}
	return nil
}

// findNodeByText find node by text.
func findNodeByText(root Node, targetText string) *Node {
	if root.Text == targetText {
		return &root
	}

	for _, child := range root.Nodes {
		if found := findNodeByText(child, targetText); found != nil {
			return found
		}
	}

	return nil
}

// findNodeByClass find node by class.
func findNodeByClass(root Node, targetClass string) *Node {
	if root.Class == targetClass {
		return &root
	}

	for _, child := range root.Nodes {
		if found := findNodeByClass(child, targetClass); found != nil {
			return found
		}
	}

	return nil
}

// swipeDown swipe screen down.
func swipeDown(ctx context.Context, dut *dut.DUT) error {
	out, err := dut.Conn().CommandContext(ctx, "adb", "shell", "getevent", "-p", "|", "grep", "-e", "\"0036\"").Output(exec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to get event 0036")
	}
	re := regexp.MustCompile(`max (\d+),`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) == 2 {
		numStr := matches[1]
		maxHeight, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return errors.Wrap(err, "failed to max height can't convert to float")
		}
		centerY := maxHeight * 0.5
		targetY := maxHeight * 0.25
		if err := dut.Conn().CommandContext(ctx, "adb", "shell", "input", "swipe", "200", strconv.FormatFloat(centerY, 'f', -1, 64), "200", strconv.FormatFloat(targetY, 'f', -1, 64), "500").Run(exec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to adb swipe to down")
		}
		return nil
	}

	return errors.New("failed to find max height")
}

// parseBounds parse bounds to center positions.
func parseBounds(bounds string) (int, int, error) {
	re := regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)
	matches := re.FindStringSubmatch(bounds)
	const boundsLen = 5
	if len(matches) == boundsLen {
		x1, _ := strconv.Atoi(matches[1])
		y1, _ := strconv.Atoi(matches[2])
		x2, _ := strconv.Atoi(matches[3])
		y2, _ := strconv.Atoi(matches[4])
		centerX := (x1 + x2) / 2
		centerY := (y1 + y2) / 2
		return centerX, centerY, nil
	}
	return 0, 0, errors.New("failed to parse bounds")
}
