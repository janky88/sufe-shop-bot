#!/bin/bash

# Test payment callback script
# Usage: ./test_callback.sh <out_trade_no> <amount>

if [ $# -lt 2 ]; then
    echo "Usage: $0 <out_trade_no> <amount>"
    echo "Example: $0 1-1234567890 4.00"
    exit 1
fi

OUT_TRADE_NO=$1
AMOUNT=$2
CALLBACK_URL="http://localhost:8080/payment/epay/notify"

# Generate test parameters
PID="test_pid"
TRADE_NO="TEST$(date +%s)"
TYPE="alipay"
NAME="Test Product"
TRADE_STATUS="TRADE_SUCCESS"

# Build parameter string for signing
PARAMS="money=${AMOUNT}&name=${NAME}&out_trade_no=${OUT_TRADE_NO}&pid=${PID}&trade_no=${TRADE_NO}&trade_status=${TRADE_STATUS}&type=${TYPE}"

# Calculate MD5 sign (params + key)
# Using "test_key" as the secret key
SIGN_STR="${PARAMS}test_key"
SIGN=$(echo -n "$SIGN_STR" | md5sum | cut -d' ' -f1)

echo "Testing payment callback..."
echo "Out Trade No: $OUT_TRADE_NO"
echo "Amount: $AMOUNT"
echo "Sign: $SIGN"
echo ""

# Send POST request
curl -X POST \
    -d "pid=${PID}" \
    -d "trade_no=${TRADE_NO}" \
    -d "out_trade_no=${OUT_TRADE_NO}" \
    -d "type=${TYPE}" \
    -d "name=${NAME}" \
    -d "money=${AMOUNT}" \
    -d "trade_status=${TRADE_STATUS}" \
    -d "sign=${SIGN}" \
    -d "sign_type=MD5" \
    "${CALLBACK_URL}"

echo ""
echo "Response received."