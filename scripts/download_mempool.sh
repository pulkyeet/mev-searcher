#!/bin/bash

# Download Flashbots mempool-dumpster data (daily files)
# Usage: ./download_mempool.sh START_DATE [END_DATE]
# Example: ./download_mempool.sh 2023-08-15 2023-08-17

START_DATE=$1
END_DATE=${2:-$START_DATE}  # If no end date, just download start date
OUTPUT_DIR="../data/mempool"

if [ -z "$START_DATE" ]; then
    echo "Usage: $0 START_DATE [END_DATE]"
    echo "Example: $0 2023-08-15"
    echo "Example: $0 2023-08-15 2023-08-17  # Downloads 3 days"
    echo "Note: Data available from 2023-08-07 onwards"
    exit 1
fi

mkdir -p $OUTPUT_DIR

current=$START_DATE
while [ "$current" != "$(date -I -d "$END_DATE + 1 day")" ]; do
    year=$(echo $current | cut -d'-' -f1)
    month=$(echo $current | cut -d'-' -f2)

    url="https://mempool-dumpster.flashbots.net/ethereum/mainnet/$year-$month/$current.parquet"
    output="$OUTPUT_DIR/$current.parquet"

    if [ -f "$output" ]; then
        echo "✓ $current.parquet already exists, skipping"
    else
        echo "📥 Downloading $current.parquet (~700MB)..."
        wget -q --show-progress "$url" -O "$output"

        if [ $? -eq 0 ] && [ -s "$output" ]; then
            echo "✓ Download successful!"
        else
            echo "✗ Download failed for $current"
            rm -f "$output"
        fi
    fi

    current=$(date -I -d "$current + 1 day")
done

echo ""
echo "✅ Download complete!"
ls -lh $OUTPUT_DIR/*.parquet