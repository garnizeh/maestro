#!/bin/bash
set -e

# Maestro Smoke Test Script (Rootless Optimized)
# This script performs a full functional validation of the Maestro container manager.

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}==> Maestro Smoke Test Start${NC}"

# 0. Pre-cleanup (ensure no stale container from a previous run)
echo -e "${YELLOW}[0/6] Pre-cleanup (removing stale container if any)...${NC}"
./bin/maestro container stop smoke-fire-test 2>/dev/null || true
./bin/maestro container rm smoke-fire-test 2>/dev/null || true

# 1. Environment Check
echo -e "${YELLOW}[1/6] Checking Environment...${NC}"
./bin/maestro version
if ! which crun > /dev/null 2>&1; then
    echo "Error: crun not found in PATH. Maestro requires an OCI runtime."
    exit 1
fi
echo -e "${GREEN}Environment OK${NC}"

# 2. Image Management (Maturin & Shardik)
echo -e "${YELLOW}[2/6] Testing Image Management...${NC}"
./bin/maestro pull alpine
./bin/maestro image ls | grep alpine
echo -e "${GREEN}Image Management OK${NC}"

# 3. Container Execution (Gan & Eld & Prim)
# Note: Using --network none to avoid unprivileged unshare(CLONE_NEWNET) failures in this environment.
echo -e "${YELLOW}[3/6] Testing Container Execution (The Fire Test)...${NC}"
./bin/maestro run --network none --name smoke-fire-test alpine echo "Maestro: The Tower Rises"
echo -e "${GREEN}Execution Finished${NC}"

# 4. State & Logging (Waystation)
echo -e "${YELLOW}[4/6] Testing Container State & Logs...${NC}"
./bin/maestro ps -a | grep smoke-fire-test
# ./bin/maestro container inspect smoke-fire-test > /dev/null
echo -e "${BLUE}Logs Output:${NC}"
./bin/maestro container logs smoke-fire-test
echo -e "${GREEN}State & Logs OK${NC}"

# 5. Cleanup
echo -e "${YELLOW}[5/6] Testing Lifecycle Cleanup...${NC}"
./bin/maestro container rm smoke-fire-test
if ./bin/maestro ps -a | grep smoke-fire-test; then
    echo "Error: Container smoke-fire-test was not removed correctly."
    exit 1
fi
echo -e "${GREEN}Cleanup OK${NC}"

# 6. Final State Check
echo -e "${YELLOW}[6/6] Final State Verification...${NC}"
./bin/maestro ps -a
echo -e "${GREEN}Final State Clean${NC}"

echo -e "${GREEN}==> Maestro Smoke Test Successfully Completed!${NC}"
