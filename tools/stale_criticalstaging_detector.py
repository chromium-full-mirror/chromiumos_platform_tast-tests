#!/usr/bin/python3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import json
import time

from google.cloud import bigquery

# Construct a BigQuery client object.


class Query():

    def __init__(self):
        self.client = bigquery.Client(project="chromeos-bot")

    def make_backbone_query(self):
        query = """

WITH
  test_num_per_build AS(
      SELECT
        build_version as build,
        COUNT(*) AS C
      FROM
        `cros-test-analytics.resultdb.cros_test_results`
      WHERE
          status NOT IN UNNEST(['NOT_RUN', 'TEST_NA'])
          AND (
            suite = 'bvt-tast-criticalstaging')
          AND duration IS NOT NULL
      GROUP BY build
      ORDER BY build DESC
      LIMIT 21
  ),
  recent_test_num_per_build_avg AS(
    SELECT
      AVG(test_num_per_build.C) AS avg
    FROM
      test_num_per_build
  ),
  all_recent_build AS(
      SELECT
        build_version as build,
        COUNT(*) AS C
      FROM
        `cros-test-analytics.resultdb.cros_test_results`
      WHERE
          status NOT IN UNNEST(['NOT_RUN', 'TEST_NA'])
          AND (
            suite = 'bvt-tast-criticalstaging')
          AND duration IS NOT NULL
      GROUP BY build
      ORDER BY build DESC
  ),
  recent_valid_21_builds AS(
    SELECT
      all_recent_build.build,
      all_recent_build.C
    FROM
      all_recent_build, recent_test_num_per_build_avg
    WHERE
      all_recent_build.C > (recent_test_num_per_build_avg.avg / 4)
    GROUP BY build, C
    ORDER BY build DESC
    LIMIT 21
  ),
  target_test AS(
      SELECT
        test,
        build_version AS build,
        queued_date as date,
        COUNT(*) AS C
      FROM
        `cros-test-analytics.resultdb.cros_test_results`
      WHERE
          status NOT IN UNNEST(['NOT_RUN', 'TEST_NA'])
          AND (
            suite = 'bvt-tast-criticalstaging')
          AND duration IS NOT NULL
      GROUP BY build, date,test
      ORDER BY date DESC
  ),
  test_build_check AS(
    SELECT
      target_test.build,
      target_test.test,
    FROM target_test, recent_valid_21_builds
    WHERE target_test.build = recent_valid_21_builds.build
  ),
  test_build_count AS(
  SELECT
    test,
    COUNT(DISTINCT build) AS C
    FROM
      test_build_check
    GROUP BY
      test
  )

SELECT
  test,
FROM
  test_build_count
WHERE
  test_build_count.C >= 21

        """
        return self.client.query(query)  # Make an API request.
