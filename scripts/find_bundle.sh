#!/bin/bash
# Find consecutive txs from same address in a block

BLOCK=$1

if [ -z "$BLOCK" ]; then
    echo "Usage: ./find_bundle.sh <block_number>"
    exit 1
fi

echo "Scanning block $BLOCK for consecutive transactions from same sender..."

# This will output tx hashes that are consecutive from same sender
go run scripts/find_consecutive.go $BLOCK