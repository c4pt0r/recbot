#!/usr/bin/env python3
# Usage:
# ./poll.py --host <xxx.tidbcloud.com> --port 4000 --user <username> --password <pwd> | jq --unbuffered .text | grep http
import signal
import time
import os
import argparse
import MySQLdb

parser = argparse.ArgumentParser(description='polls db for new rows')
parser.add_argument('--host', help='db host', default='localhost')
parser.add_argument('--port', help='port', default='4000')
parser.add_argument('--user', help='db user', default='root')
parser.add_argument('--password', help='db password', default='')
parser.add_argument('--db', help='db name', default='test')
parser.add_argument('--interval', help='polling interval in seconds', default='1')
parser.add_argument('--skip-exists', help='skip the first batch, only polling for new rows', action=argparse.BooleanOptionalAction, default=False)
args = parser.parse_args()

connection = MySQLdb.connect(host=args.host, 
                             port=int(args.port), 
                             user=args.user,
                             passwd=args.password,
                             db=args.db)
connection.autocommit(True)

def handler(signum, frame):
    print("Signal handler called with signal", flush=True)
    connection.close()
    exit(0)

signal.signal(signal.SIGINT, handler)
signal.signal(signal.SIGTERM, handler)

max_id = 0
# first run
cursor = connection.cursor()
cursor.execute("SELECT id, content FROM test.recbot ORDER BY id DESC LIMIT 100")
m = cursor.fetchall()
for row in m:
    if row[0] > max_id:
        max_id = row[0]
    if not args.skip_exists:
        print(row[1], flush=True)
cursor.close()

# polling
while True:
    stmt = "SELECT id, content FROM test.recbot WHERE id > %s ORDER BY id DESC LIMIT 100" % (max_id,)
    cursor = connection.cursor()
    cursor.execute(stmt)
    m = cursor.fetchall()
    for row in m:
        if row[0] > max_id:
            max_id = row[0]
        print(row[1], flush=True)
    cursor.close()
    time.sleep(int(args.interval))
