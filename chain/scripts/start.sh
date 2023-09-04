#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

docker compose up celestia-devnet -d --wait

# Initialize DA_AUTH_TOKEN to an empty value
export DA_AUTH_TOKEN=""

# Define the command to extract and set DA_AUTH_TOKEN
extract_command='export DA_AUTH_TOKEN=$(docker logs celestia_devnet 2>&1 | grep CELESTIA_NODE_AUTH_TOKEN -A 5 | tail -n 1)'

# Loop until DA_AUTH_TOKEN is set
while [ -z "$DA_AUTH_TOKEN" ]; do
    # Run the extract command
    eval "$extract_command"

    # Check if DA_AUTH_TOKEN is set
    if [ -n "$DA_AUTH_TOKEN" ]; then
        echo "DA_AUTH_TOKEN set: $DA_AUTH_TOKEN"
    else
        echo "DA_AUTH_TOKEN is not set yet. Retrying..."
        sleep 1  # Adjust the sleep duration as needed
    fi
done

echo "starting rollup..."
docker compose up chain






