import sys
import pyarrow.parquet as pq
import pandas as pd

if len(sys.argv) < 2:
    print("Usage: python3 inspect_parquet.py <parquet_file>")
    sys.exit(1)

file_path = sys.argv[1]

print(f"Inspecting: {file_path}\n")

# Read parquet file
table = pq.read_table(file_path)
df = table.to_pandas()

print("=" * 60)
print("SCHEMA")
print("=" * 60)
print(f"Columns: {list(df.columns)}")
print(f"Total rows: {len(df)}")
print()

print("=" * 60)
print("FIRST 3 ROWS")
print("=" * 60)
print(df.head(3))
print()

print("=" * 60)
print("COLUMN TYPES")
print("=" * 60)
print(df.dtypes)
print()

print("=" * 60)
print("SAMPLE TRANSACTION")
print("=" * 60)
if len(df) > 0:
    first_tx = df.iloc[0]
    for col in df.columns:
        val = first_tx[col]
        if isinstance(val, bytes):
            print(f"{col}: <bytes, length={len(val)}>")
        else:
            print(f"{col}: {val}")
