#!/usr/bin/env python3

# Webhook Notifier — NZBGet Extension
#
# Installation:
#   1. In your NZBGet ScriptDir create a folder called WebhookNotifier
#   2. Place both manifest.json and this file (main.py) inside that folder
#   3. In NZBGet go to Settings and reload the page
#   4. Under EXTENSION MANAGER the WebhookNotifier extension should appear
#   5. Set the WebhookUrl to your NORA ingest URL
#      
#   6. Under EXTENSIONS add WebhookNotifier to the post-processing list
#   7. Save settings
#
# A failed webhook notification will never block or fail your download.

import os
import sys
import json
import urllib.request

# Exit codes used by NZBGet
POSTPROCESS_SUCCESS = 93
POSTPROCESS_ERROR   = 94
COMMAND_SUCCESS     = 93
COMMAND_ERROR       = 94

# ── Command mode (Test button in NZBGet UI) ──────────────────────────────────
command = os.environ.get('NZBCP_COMMAND')
if command is not None:
    if command == 'ConnectionTest':
        webhook_url = os.environ.get('NZBPO_WEBHOOKURL', '').strip()
        if not webhook_url:
            print('[ERROR] WebhookUrl is not set')
            sys.exit(COMMAND_ERROR)
        if not webhook_url.startswith(('http://', 'https://')):
            print(f'[ERROR] WebhookUrl "{webhook_url}" does not look like a valid URL')
            sys.exit(COMMAND_ERROR)
        try:
            test_payload = {'action': 'test', 'name': 'NZBGet connection test'}
            data = json.dumps(test_payload).encode('utf-8')
            req  = urllib.request.Request(
                webhook_url,
                data=data,
                headers={'Content-Type': 'application/json'},
                method='POST'
            )
            with urllib.request.urlopen(req, timeout=10) as resp:
                print(f'[INFO] Connection test successful ({resp.status})')
            sys.exit(COMMAND_SUCCESS)
        except Exception as e:
            print(f'[ERROR] Connection test failed — {e}')
            sys.exit(COMMAND_ERROR)
    else:
        print(f'[ERROR] Unknown command: {command}')
        sys.exit(COMMAND_ERROR)

# ── Post-processing mode ─────────────────────────────────────────────────────
webhook_url = os.environ.get('NZBPO_WEBHOOKURL', '').strip()

if not webhook_url:
    print('[INFO] WebhookUrl not set — skipping notification')
    sys.exit(POSTPROCESS_SUCCESS)

if not webhook_url.startswith(('http://', 'https://')):
    print(f'[WARNING] WebhookUrl "{webhook_url}" does not look like a valid URL — skipping notification')
    sys.exit(POSTPROCESS_SUCCESS)

# Read download info from NZBGet environment variables
name         = os.environ.get('NZBPP_NZBNAME', 'Unknown')
category     = os.environ.get('NZBPP_CATEGORY', '')
total_status = os.environ.get('NZBPP_TOTALSTATUS', 'UNKNOWN')
raw_status   = os.environ.get('NZBPP_STATUS', '')

# Parse NZBPP_STATUS — format is always TOTALSTATUS/DETAIL e.g. FAILURE/PAR
status_parts  = raw_status.split('/', 1)
status_detail = status_parts[1] if len(status_parts) == 2 else ''

# Map NZBGet total status to webhook action
status_map = {
    'SUCCESS': 'download.success',
    'WARNING': 'download.warning',
    'FAILURE': 'download.failure',
    'DELETED': None,
}

action = status_map.get(total_status)

if action is None:
    print(f'[INFO] Skipping status {total_status}')
    sys.exit(POSTPROCESS_SUCCESS)

# Build a human-readable failure reason from status_detail
failure_reason_map = {
    'PAR':        'Par-check failed',
    'UNPACK':     'Unpack failed',
    'HEALTH':     'Download health too low',
    'MOVE':       'Failed to move files',
    'SCRIPT':     'Post-processing script failed',
    'REPAIRABLE': 'Damaged but repairable — manual action needed',
    'SPACE':      'Not enough disk space',
    'PASSWORD':   'Wrong password for archive',
    'DAMAGED':    'Download is damaged',
    'SCAN':       'Scan failed',
}
failure_reason = failure_reason_map.get(status_detail, status_detail)

payload = {
    'action':         action,
    'name':           name,
    'category':       category,
    'status':         total_status,
    'status_detail':  status_detail,    # raw e.g. PAR, UNPACK, PASSWORD
    'failure_reason': failure_reason,   # human readable — empty on success
}

try:
    data = json.dumps(payload).encode('utf-8')
    req  = urllib.request.Request(
        webhook_url,
        data=data,
        headers={'Content-Type': 'application/json'},
        method='POST'
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        print(f'[INFO] Webhook sent — {action} ({resp.status})')
except Exception as e:
    print(f'[WARNING] Failed to send webhook — {e}')

sys.exit(POSTPROCESS_SUCCESS)
