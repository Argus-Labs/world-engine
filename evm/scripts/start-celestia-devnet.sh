#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

docker compose up celestia-devnet -d --wait

# Initialize DA_AUTH_TOKEN to an empty value
export DA_AUTH_TOKEN=""

extract_command='export DA_AUTH_TOKEN=$(docker exec $(docker ps -q) celestia bridge auth admin --node.store /home/celestia/bridge)'

# Loop until DA_AUTH_TOKEN is set
while [ -z "$DA_AUTH_TOKEN" ]; do
    # Run the extract command
    eval "$extract_command"

    # Check if DA_AUTH_TOKEN is set
    if [ -n "$DA_AUTH_TOKEN" ]; then
        echo "DA_AUTH_TOKEN set: $DA_AUTH_TOKEN"
        sleep 5 # avoid race condition, let devnet finish starting
    else
        echo "DA_AUTH_TOKEN is not set yet. Retrying..."
        sleep 2  # Adjust the sleep duration as needed
    fi
done
