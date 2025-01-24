#!/bin/bash

BASE_URL="http://localhost:3001"
STATE_NAME="test-state"
STATE_FILE="state.tfstate"

echo "--- Testing POST method (create state) ---"
curl -X POST "$BASE_URL/$STATE_NAME" -d '{"version": 1, "terraform_version": "1.4.0"}' -H "Content-Type: application/json"

echo "--- Testing GET method (retrieve state) ---"
curl -X GET "$BASE_URL/$STATE_NAME"

echo "--- Testing LOCK method ---"
curl -X LOCK "$BASE_URL/$STATE_NAME"

echo "--- Testing DELETE method (locked state) ---"
curl -X DELETE "$BASE_URL/$STATE_NAME"

echo "--- Testing UNLOCK method ---"
curl -X UNLOCK "$BASE_URL/$STATE_NAME"

echo "--- Testing DELETE method (delete state) ---"
curl -X DELETE "$BASE_URL/$STATE_NAME"
