#!/bin/bash

# Test script to verify node API endpoint

echo "Testing node add API..."
echo ""

# First, login to get session cookie
echo "1. Logging in..."
COOKIE_JAR=$(mktemp)
curl -X POST http://192.168.10.192:2053/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' \
  -c "$COOKIE_JAR" \
  -s | jq '.'

echo ""
echo "2. Testing node add with only name field..."
curl -X POST http://192.168.10.192:2053/panel/api/node/add \
  -H "Content-Type: application/json" \
  -b "$COOKIE_JAR" \
  -d '{"name":"Test-Node"}' \
  -v 2>&1 | grep -E "< HTTP|success|error|msg"

echo ""
echo "3. Testing node add with all fields..."
curl -X POST http://192.168.10.192:2053/panel/api/node/add \
  -H "Content-Type: application/json" \
  -b "$COOKIE_JAR" \
  -d '{"name":"Test-Node","address":"","port":0,"secret":""}' \
  -s | jq '.'

rm -f "$COOKIE_JAR"
