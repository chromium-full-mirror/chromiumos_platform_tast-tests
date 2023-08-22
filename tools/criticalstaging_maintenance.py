#!/usr/bin/python3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Tool for auto maintenance remove stale test  from Criticalstaging

    1. Use stale_criticalstaging_detector.py to generate the data and put it in a temporary json file(output.json).
    2. Find the current target test "group:critical staging" string and remove.
    3. Auto commit with following message: COMMIT_MESSAGE
    4. Upload the CL and assign the owner from the test contacts field.
    4. Set current repo back to main repo after repo upload. Delete the temp repo and temp json file.

    Environment Settings if needed:
    Outside chroot, run the following commands. skip if you have installed google-cloud or google-cloud-bigquery
    $ pip3 install google-cloud --break-system-packages
    $ pip3 install google-cloud-bigquery --break-system-packages

    Authorize and set the project as chromeos-bot:
    $ gcloud auth application-default login
    $ gcloud auth application-default set-quota-project chromeos-bot

    Usage example:
    $ python criticalstaging_maintenance.py

    Contributor:
    dbeckett@google.com
"""

import os
import subprocess
import json
import re
import time
import logging

PATTERN = re.compile(r'(?<!^)(?=[A-Z])')
FUNC_MATCH = 'func init() {'
COMMIT_MESSAGE = """
This CL is Automatically generated. This tests has been detected by Criticalstaging maintenance script which already in criticalStaging group more than 21 builds (14 is max limit)

The CriticalStaging group is used to indicate that a test is intended to go into "mainline" critical testing. This group will be run on all boards/models, only on ToT. Tests can only remain in this group long enough to gather signal (10 consecutive builds), after which the owner must promote them into mainline only, or back into informational. If no owner action is taken after a 4 builds grace period, they will be moved into informational.

If more data is needed for test, please create a new CL to re-add it to the group.

BUG={bug}
TEST=cq
"""


def run_cmd(cmd):
    p = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    _, error = p.communicate()
    return p.returncode, error


def commit(testgroup, contact):
    """
    Upload the CL and assign the owner from the test contacts field.

    Args:
        testgroup: Name of the test group.
        contacts: Owners of the target test.
    """
    def run_flow(testgroup, contacts, bug="None"):
        returncode, error = run_cmd(f'repo start no_stale_cs_{testgroup}_tests')
        if returncode != 0:
            raise Exception("The command 'repo start no_stale_cs_{testgroup}_tests' failed:", error)

        returncode, error = run_cmd(f'git add src/.')
        if returncode != 0:
            raise Exception("The command 'git add' failed:", error)

        returncode, error = run_cmd(f'git commit -m "Remove stale criticalStaging tests" -m "{COMMIT_MESSAGE.format(bug=bug)}"')
        if returncode != 0:
            raise Exception("The command 'git commit' failed:", error)

        returncode, error = run_cmd(f'repo upload . --no-verify --re={contacts} --cc=seewaifu@google.com -y')
        if returncode != 0:
            raise Exception("The command 'repo upload' failed:", error)

        returncode, error = run_cmd('repo abandon no_stale_cs_{testgroup}_tests')
        if returncode != 0:
            raise Exception("The command 'repo abandon no_stale_cs_{testgroup}_tests' failed:", error)

    run_flow(testgroup, contact)


def find_test_hardway(t):
    tdir = t.split('.')[0]
    fname = []
    for root, _, f_names in os.walk(os.path.dirname(os.getcwd())):
        for f in f_names:
            fname = (os.path.join(root, f))

            if tdir in fname and ".go" in fname:
                with open(fname, 'r') as rf:
                    lines = rf.readlines()

                _, func_line = get_attr_lines(lines)
                tfunc = t.split('.')[1]
                if str(func_line).replace(',','') == str(tfunc).strip():
                    return fname
    return None


def walker(t):
    tdir = t.split('.')[0]
    tname = t.split('.')[1]
    name = f'{PATTERN.sub("_", tname).lower()}.go'
    fname = []
    for root, _, f_names in os.walk(os.path.dirname(os.getcwd())):
        for f in f_names:
            fname = (os.path.join(root, f))

            if tdir in fname and name in fname:
                return fname
    return None


def get_attr_lines(lines):
    """
    Get target test owner information after string 'Contacts:' found.
    The search range is limited between two braces of test init function 'init() {}'.
    to prevent searching in the test other functions.

    Args:
        lines: each line of target test.
    """
    func_init_found = False
    contact_line = None
    func_line = None
    for i, line in enumerate(lines):
        # Find test function init point
        if line.startswith(FUNC_MATCH):
            func_init_found = True
        # Stop at the end of init function
        if func_init_found and line == '}\n':
            break
        # Get owner information after Contacts:
        if func_init_found and 'Contacts' in line:
            contact_line = line.split(':')[-1].strip()
            if '}' not in contact_line:
                last_line_index = 0
                cLines = [contact_line]
                for current_index in range(i, len(lines)):
                    if '}' in lines[current_index]:
                        last_line_index = current_index
                        break
                for current_index in range(i, last_line_index):
                    cLines.append(lines[current_index + 1])
                contact_line = ''.join([l.strip() for l in cLines])
        if func_init_found and 'Func:' in line:
            func_line = line.split(':')[-1].strip()

    return contact_line, func_line


def replace_data(data):
    """
    Replace group:criticalstaging with empty string.
    It is necessary to replace the case which comma before target string first, do not change the order.

    Args:
        data: input and output with replace string.
    """
    # Order matters here.
    data = data.replace(', "group:criticalstaging"', "")
    data = data.replace('"group:criticalstaging", ', "")
    data = data.replace('"group:criticalstaging"', "")
    return data


def remove_criticalstaging_attr(t):
    """
    Find the current target test "group:criticalstaging" string and remove.

    Args:
        t: target test.
    """
    logger = logging.getLogger()
    testfn = walker(t)
    if not testfn:
        testfn = find_test_hardway(t)
        if not testfn:
            logger.info('Can not find target test {}'.format(t))
            return 1, []

    with open(testfn, 'r') as rf:
        data = rf.read()
        lines = data.splitlines()
        replaced_data = replace_data(data)

    with open(testfn, 'w') as file:
        file.write(replaced_data)

    contacts, _ = get_attr_lines(lines)

    return contacts


def get_name_group(name):
    """
    Return test name and group.
    Example: tast.lacros.DmaBufLeak return name: lacros.DmaBufLeak, group:lacros
    """
    if 'tast.' in name:
        name = name.split('tast.')[-1]
        group = name.split('.')[0]
        return name, group
    return "", ""


def format_emails(emails):
    return ",".join(list(emails))


def get_tests_with_groups():
    """
    Return dictionaries contain key as group name, value as target tests.
    Example:
    {
    'arc': ['arc.AppManagement', 'arc.Boot.vm_virtio_blk_data_o_direct', 'arc.ManagedPlayAvailableAppUninstall.vm'],
    'lacros': ['lacros.DmaBufLeak', 'lacros.MigrateExtensionState', 'lacros.MigrateBasic'],
    'ui': ['ui.WebUIJSErrors.lacros']
    }
    """
    with open('output.json') as f:
        data = json.load(f)

    tests_with_groups = {}
    for major in data:
        for test in major:
            name, group = get_name_group(test)
            if not name:
                continue
            if group not in tests_with_groups:
                tests_with_groups[group] = [name]
            else:
                tests_with_groups[group].append(name)
    return tests_with_groups


def generate_stale_tests_data():
    """
    Use stale_criticalstaging_detector to generate the data and put it in a temporary json file.
    """
    from stale_criticalstaging_detector import Query
    query = Query()
    query_job = query.make_backbone_query()
    majors = ["test"]

    with open('output.json', 'w') as wf:
        for row in query_job:
            majors.append([row['test']])
        json.dump(majors, wf)


def cleanup_stale_tests_data():
    returncode, error = run_cmd('rm -f output.json')
    if returncode != 0:
        raise Exception("The command 'rm -f output.json' failed:", error)


def main():
    logging.basicConfig(level=logging.INFO)
    logger = logging.getLogger()
    generate_stale_tests_data()
    tests_with_groups = get_tests_with_groups()
    cleanup_stale_tests_data()
    for testGroup in tests_with_groups:
        tests = tests_with_groups[testGroup]
        ccSet = set()
        for test in tests:
            contacts = remove_criticalstaging_attr(test)
            cs = re.findall(r'"(.*?)"', contacts)
            ccSet.update(set(cs))
        logger.info('Would commit: {}'.format(testGroup))
        commit(testGroup, format_emails(ccSet))


if __name__ == '__main__':
    main()