#!/bin/bash
# Verify that kubefwd has cleaned up properly after tests
# Usage: ./verify-cleanup.sh

set -e

echo "========================================="
echo "Verifying kubefwd cleanup"
echo "========================================="

CLEANUP_OK=true

# Check for kubefwd processes
echo ""
echo "Checking for kubefwd processes..."
if pgrep -f kubefwd >/dev/null 2>&1; then
  echo "ERROR: kubefwd processes still running:"
  pgrep -af kubefwd
  CLEANUP_OK=false
else
  echo "✓ No kubefwd processes running"
fi

# Check for lingering loopback aliases (127.1.x.x)
echo ""
echo "Checking for loopback IP aliases (127.1.x.x)..."

# Detect platform
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS
  if ifconfig lo0 2>/dev/null | grep -q "inet 127\.1\."; then
    echo "WARNING: Loopback aliases still present on lo0:"
    ifconfig lo0 | grep "inet 127\.1\."
    CLEANUP_OK=false
  else
    echo "✓ No 127.1.x.x aliases found on lo0"
  fi
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
  # Linux
  if ip addr show lo 2>/dev/null | grep -q "inet 127\.1\."; then
    echo "WARNING: Loopback aliases still present on lo:"
    ip addr show lo | grep "inet 127\.1\."
    CLEANUP_OK=false
  else
    echo "✓ No 127.1.x.x aliases found on lo"
  fi
else
  echo "WARNING: Unknown OS type, skipping loopback alias check"
fi

# Check /etc/hosts for kubefwd entries
echo ""
echo "Checking /etc/hosts for kubefwd entries..."
if grep -q "# kubefwd" /etc/hosts 2>/dev/null || grep -q "127\.1\." /etc/hosts 2>/dev/null; then
  echo "WARNING: Possible kubefwd entries still in /etc/hosts:"
  grep -E "(# kubefwd|127\.1\.)" /etc/hosts | head -5
  CLEANUP_OK=false
else
  echo "✓ No obvious kubefwd entries in /etc/hosts"
fi

echo ""
echo "========================================="
if [ "$CLEANUP_OK" = true ]; then
  echo "✓ Cleanup verification PASSED"
  echo "========================================="
  exit 0
else
  echo "✗ Cleanup verification FAILED"
  echo "========================================="
  echo ""
  echo "Manual cleanup may be required:"
  echo "  - Kill kubefwd processes: pkill -f kubefwd"
  echo "  - Remove IP aliases (macOS): sudo ifconfig lo0 -alias 127.1.x.x"
  echo "  - Remove IP aliases (Linux): sudo ip addr del 127.1.x.x/8 dev lo"
  echo "  - Restore /etc/hosts from backup: ~/hosts.original"
  exit 1
fi
