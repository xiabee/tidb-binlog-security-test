#!/bin/sh

set -e

cd "$(dirname "$0")"

args="-initial-commit-ts=-1"

kafka_addr=${KAFKA_ADDRS-127.0.0.1:9092}

run_drainer "$args" &

GO111MODULE=on go build -o out

./out -offset=-1 -topic=binlog_test_topic -kafkaAddr=$kafka_addr -> ${OUT_DIR-/tmp}/$TEST_NAME.out 2>&1

killall drainer
