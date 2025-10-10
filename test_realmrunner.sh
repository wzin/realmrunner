#!/bin/bash
set -e

echo "Testing RealmRunner..."
echo ""

# Test 1: Check if container is running
echo "Test 1: Container status..."
if docker compose ps | grep -q "realmrunner.*Up"; then
    echo "✓ Container is running"
else
    echo "✗ Container is not running"
    exit 1
fi

# Test 2: Check if web server is responding
echo ""
echo "Test 2: Web server health..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/)
if [ "$HTTP_CODE" = "200" ]; then
    echo "✓ Web server is responding (HTTP $HTTP_CODE)"
else
    echo "✗ Web server returned HTTP $HTTP_CODE"
    exit 1
fi

# Test 3: Fetch and verify index.html contains expected content
echo ""
echo "Test 3: Fetching index.html..."
RESPONSE=$(curl -s http://localhost:8080/)
if echo "$RESPONSE" | grep -q "RealmRunner"; then
    echo "✓ Index.html contains 'RealmRunner'"
else
    echo "✗ Index.html does not contain expected content"
    exit 1
fi

# Test 4: Check if API endpoint exists (should return 401 without auth)
echo ""
echo "Test 4: API endpoint accessibility..."
API_CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/api/servers)
if [ "$API_CODE" = "401" ]; then
    echo "✓ API is responding (HTTP $API_CODE - auth required as expected)"
else
    echo "✗ API returned unexpected status: HTTP $API_CODE"
    exit 1
fi

echo ""
echo "============================================"
echo "✓ All tests passed!"
echo "============================================"
