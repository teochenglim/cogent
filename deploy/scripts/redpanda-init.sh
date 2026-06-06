#!/bin/sh
set -e
echo "Creating cogent-telemetry topic..."
rpk topic create cogent-telemetry \
  --partitions 8 \
  --replicas 1 \
  --topic-config retention.ms=604800000
echo "Topic created."
