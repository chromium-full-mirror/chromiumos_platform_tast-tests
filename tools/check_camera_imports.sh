#!/bin/bash
#
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Receives ${PRESUBMIT_COMMIT} on the command line and checks that if
# "CAMERA_DEPENDENTS_CHECK=true" presents in the commit message if any new
# dependencies to camera utilities are introduced.

commit="$1"
new_camera_utils_dependency=\
'^\+\s+\"go.chromium.org/tast-tests/cros/local/camera.*\"'
camera_dependents_check='CAMERA_DEPENDENTS_CHECK=true'

new_dependency_found=$(git show --unified=0 --pretty="" "${commit}" | \
    grep -E "${new_camera_utils_dependency}")
check_in_commit_message=$(git show --no-patch "${commit}" | \
    grep "${camera_dependents_check}")

if [[ -n "${new_dependency_found}" ]] \
    && [[ -z "${check_in_commit_message}" ]]; then
  echo "When adding dependency to camera utilities, please ensure that the "
  echo "dependents tests are covered by \"camera_cq\" test plan so that any "
  echo "changes on camera utilites will also test the dependent tests. "
  echo "To include the test into the test plan, an easy way is to add "
  echo "\"group:camera_dependent\" into the test attribute. "
  echo "Once the check is done, please add \"${camera_dependents_check}\" in "
  echo "the commit message."
  exit 1
fi
