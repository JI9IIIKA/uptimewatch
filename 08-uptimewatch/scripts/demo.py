#!/usr/bin/env python3
"""Exercise one running project. Run from the repository root after setup.py."""
import argparse
import datetime as dt
import json
from pathlib import Path
import time
import urllib.request
import urllib.error

parser = argparse.ArgumentParser()
parser.add_argument('project', choices=['uptimewatch'])
parser.add_argument('--url', default='http://localhost:8087')
args = parser.parse_args()
values = dict(line.split('=',1) for line in (Path(__file__).resolve().parent.parent/'.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
token = values['ADMIN_TOKEN']


def call(method, path, body=None):
    req = urllib.request.Request(args.url+path, data=None if body is None else json.dumps(body).encode(), method=method,
        headers={'Authorization': 'Bearer '+token,'Content-Type':'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            result = json.load(response)
    except urllib.error.HTTPError as error:
        raise SystemExit(f'{method} {path}: {error.code} {error.read().decode()}')
    print(method, path, json.dumps(result, ensure_ascii=False))
    return result


project=args.project
if project=='uptimewatch':
    monitor=call('POST','/monitors',{'url':'https://example.com','interval_seconds':60,'threshold':2})
    time.sleep(7)
    call('GET','/monitors/'+monitor['id']+'/checks')
