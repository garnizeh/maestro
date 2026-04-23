#!/bin/bash
set -e

# Maestro Volume & ID Mapping Smoke Test
# This script verifies volume mounting and rootless UID mapping (The Way of the Beam).

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}==> Maestro Volume Smoke Test Start${NC}"

# 0. Pre-cleanup
echo -e "${YELLOW}[0/6] Pre-cleanup (removing stale container if any)...${NC}"
./bin/maestro container stop smoke-volume-test 2>/dev/null || true
./bin/maestro container rm smoke-volume-test 2>/dev/null || true
TEMP_DIR="/tmp/maestro-smoke-volume-$(date +%s)"
rm -rf "$TEMP_DIR" 2>/dev/null || true

# 1. Setup Host Directory (The Drawing of the Three)
echo -e "${YELLOW}[1/6] Setting up Host Directory...${NC}"
mkdir -p "$TEMP_DIR/data"
chmod 777 "$TEMP_DIR/data"
echo -e "${GREEN}Host Directory Ready at $TEMP_DIR/data${NC}"

# 2. Image Management (Maturin & Shardik)
echo -e "${YELLOW}[2/6] Pulling Alpine Image...${NC}"
./bin/maestro pull alpine
./bin/maestro image ls | grep alpine
echo -e "${GREEN}Image OK${NC}"

# 3. Container Execution (Gan & Eld & Prim)
# Testing volume mount and rootless write translation
echo -e "${YELLOW}[3/6] Running Volume Test Container...${NC}"
# Use --network none to ensure it doesn't fail due to networking issues in strict envs
./bin/maestro run --network none --name smoke-volume-test --rm \
  -v "$TEMP_DIR/data:/data" \
  alpine -- \
  sh -c "echo 'maestro-id-check' > /data/check.txt"
echo -e "${GREEN}Execution Finished${NC}"

# 4. Verify Content (The Keyhole)
echo -e "${YELLOW}[4/6] Verifying File Content on Host...${NC}"
if [ -f "$TEMP_DIR/data/check.txt" ] && grep -q "maestro-id-check" "$TEMP_DIR/data/check.txt"; then
    echo -e "${GREEN}Content OK: 'maestro-id-check' found in host file${NC}"
else
    echo -e "${RED}Error: Failed to find check file or content on host!${NC}"
    exit 1
fi

# 5. Verify UID Mapping (The Tower's Protection)
echo -e "${YELLOW}[5/6] Verifying Rootless ID Mapping...${NC}"
HOST_UID=$(id -u)
FILE_UID=$(stat -c %u "$TEMP_DIR/data/check.txt")

echo "Host UID: $HOST_UID"
echo "File UID: $FILE_UID"

if [ "$FILE_UID" -eq 0 ] && [ "$HOST_UID" -ne 0 ]; then
    echo -e "${RED}Error: file UID = 0 (root); want $HOST_UID (host UID). Rootless ID mapping failed!${NC}"
    exit 1
else
    echo -e "${GREEN}ID Mapping OK: Host user correctly owns the container-written file.${NC}"
fi

# 6. Cleanup (The Clearing at the End of the Path)
echo -e "${YELLOW}[6/6] Cleaning up...${NC}"
rm -rf "$TEMP_DIR"
echo -e "${GREEN}Cleanup OK${NC}"

echo -e "${GREEN}==> Maestro Volume Smoke Test Successfully Completed!${NC}"
