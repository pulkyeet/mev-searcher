#!/usr/bin/env python3
import sys
import sqlite3
import pyarrow.parquet as pq
from datetime import datetime

if len(sys.argv) < 3:
    print("Usage: python3 parquet_to_sqlite.py <parquet_file> <db_file>")
    sys.exit(1)

parquet_file = sys.argv[1]
db_file = sys.argv[2]

print(f"📥 Processing {parquet_file}...")

# Open parquet file
parquet_table = pq.read_table(parquet_file)
print(f"📊 Total rows: {parquet_table.num_rows}")

# Create SQLite connection
conn = sqlite3.connect(db_file)
cursor = conn.cursor()

# Drop old table and create new one WITH included_block_timestamp
cursor.execute('DROP TABLE IF EXISTS mempool_txs')
cursor.execute('''
CREATE TABLE mempool_txs (
    tx_hash TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    included_block INTEGER NOT NULL,
    included_block_timestamp INTEGER,
    raw_tx TEXT NOT NULL,
    tx_from TEXT NOT NULL,
    tx_to TEXT,
    gas_price TEXT
)
''')

# Process in chunks
chunk_size = 10000
total = 0

for i in range(0, parquet_table.num_rows, chunk_size):
    batch = parquet_table.slice(i, min(chunk_size, parquet_table.num_rows - i))
    
    # Extract needed columns
    for row_idx in range(batch.num_rows):
        # Handle datetime timestamp
        ts = batch['timestamp'][row_idx].as_py()
        if isinstance(ts, datetime):
            timestamp = int(ts.timestamp())
        else:
            timestamp = int(ts) // 1000  # ms to sec
        
        tx_hash = batch['hash'][row_idx].as_py()
        
        block = batch['includedAtBlockHeight'][row_idx].as_py()
        block = int(block) if block else 0
        
        # Get included block timestamp
        block_ts = batch['includedBlockTimestamp'][row_idx].as_py()
        if isinstance(block_ts, datetime):
            included_block_timestamp = int(block_ts.timestamp())
        elif block_ts:
            included_block_timestamp = int(block_ts) // 1000
        else:
            included_block_timestamp = None
        
        raw_tx_bytes = batch['rawTx'][row_idx].as_py()
        raw_tx = raw_tx_bytes.hex() if isinstance(raw_tx_bytes, bytes) else raw_tx_bytes
        
        tx_from = batch['from'][row_idx].as_py()
        tx_to = batch['to'][row_idx].as_py() if batch['to'][row_idx].as_py() else ""
        gas_price = batch['gasPrice'][row_idx].as_py()
        
        cursor.execute(
            'INSERT OR IGNORE INTO mempool_txs VALUES (?, ?, ?, ?, ?, ?, ?, ?)',
            (tx_hash, timestamp, block, included_block_timestamp, raw_tx, tx_from, tx_to, gas_price)
        )
    
    total += batch.num_rows
    if total % 50000 == 0:
        print(f"  ✓ Processed {total:,} txs...")
        conn.commit()

conn.commit()
print("📇 Creating indexes...")
cursor.execute('CREATE INDEX IF NOT EXISTS idx_block ON mempool_txs(included_block)')
cursor.execute('CREATE INDEX IF NOT EXISTS idx_timestamp ON mempool_txs(timestamp)')
cursor.execute('CREATE INDEX IF NOT EXISTS idx_block_timestamp ON mempool_txs(included_block_timestamp)')
conn.commit()
conn.close()

print(f"✅ Done! Ingested {total:,} transactions")