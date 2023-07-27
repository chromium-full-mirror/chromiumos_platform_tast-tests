#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# Copyright 2022 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Tool for getting and saving ChromePolicy API policy schemas from the TAPE
server.

Used to create policy_schemas.json.

Generating a new policy_schemas.json file:
  Running the script requires the client secret of the default service account
  of TAPE. If you don't have access to it contact cros-engprod-muc@google.com
  to regenerate policy_schemas.json for you.

  Otherwise run the script and follow the prompts. You will be requested to
  follow a link to generate an authorization code and paste it for use in the
  script.
  > ./get_policy_schemas.py --tape_client_secret <secret>
  Use a CL to check in the updated file.
"""

import argparse
import json
import os
import requests

# Default output filename and path.
OUTPUT_FILENAME = 'policy_schemas'
OUTPUT_FILEPATH = os.path.join(os.path.dirname(__file__), "..",
  OUTPUT_FILENAME)

# URLS
POLICY_SCHEMAS_URL = 'https://chromepolicy.googleapis.com/v1/customers/C02gxaaci/policySchemas?pageSize=100&pageToken='

OAUTH_RETRIEVE_TEXT = 'To retrieve one you can use OAuth Playground: https://developers.google.com/oauthplayground\nUse https://www.googleapis.com/auth/chrome.management.policy as scope click \"Authorize APIs\" and select a managedchrome account for authorization. Then press the \"Exchange authorization code for tokens\" button and use the \"Access token\" as the --oauth2 parameter for this script.'
OAUTH_HELP_TEXT = 'OAuth2 token for authorization against ChromePolicy API. ' + OAUTH_RETRIEVE_TEXT
OAUTH_MISSING_TEXT = 'Please provide an OAuth2 token. ' + OAUTH_RETRIEVE_TEXT

def raise_response_error(key, response, source):
  """Raise an error that a given key was not found in a response."""
  raise KeyError(f'Could not find {key} in response {response} from {source}')

def main():
  parser = argparse.ArgumentParser()
  parser.add_argument('--out', dest='out', default=OUTPUT_FILEPATH, type=str,
    help=('Optional filepath for the output. By default, '
    'use the path where the output is checked in.'))
  parser.add_argument('--oauth2', dest='oauth2',
    type=str, help= (OAUTH_HELP_TEXT))

  args = parser.parse_args()
  if not args.oauth2:
    print(OAUTH_MISSING_TEXT)
    return

  # Make calls to the ChromePolicy API to retrieve all policy schemas.
  file_index = 0
  page_token = ''
  auth_token = 'Bearer ' + args.oauth2
  headers = {'Authorization': auth_token}
  while True:
    with requests.get(POLICY_SCHEMAS_URL+page_token, headers=headers) as r:
      try:
        policy_schema_json = r.json()
      except:
        return
      if 'policySchemas' not in r.json():
        raise_response_error('policySchemas', r.json(), POLICY_SCHEMAS_URL)

    # Write policy schemas to the file.
    policy_schemas = (r.json()['policySchemas'])
    file_name = args.out + "_{:02d}".format(file_index) + '.json'
    with open(file_name, 'w') as fh:
      fh.write('[')
      for policy_schema in policy_schemas[:-1]:
        fh.write(json.dumps(policy_schema, indent=2))
        fh.write(',')
      fh.write(json.dumps(policy_schemas[-1], indent=2))
      fh.write(']')
      file_index+=1

      if 'nextPageToken' in r.json():
        page_token = r.json()['nextPageToken']
      else:
        break

if __name__ == '__main__':
  main()
