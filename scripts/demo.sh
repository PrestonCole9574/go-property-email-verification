#!/bin/sh
set -eu

curl --fail-with-body --silent --show-error \
  -X POST http://localhost:8080/signups \
  -H 'Content-Type: application/json' \
  -d '{"property_id":"building-7","tenant_id":"tenant-42","email":"tenant@example.com"}'
printf '\n'
