#!/bin/bash
set -e

# Maestro Nginx Smoke Test (Rootless Networking Validation)
# This script verifies the full networking stack: pull, run, port mapping, and curl.

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}==> Maestro Nginx Smoke Test Start${NC}"

# 0. Pre-cleanup (ensure no stale container from a previous run)
echo -e "${YELLOW}[0/6] Pre-cleanup (removing stale container if any)...${NC}"
./bin/maestro container stop smoke-nginx 2>/dev/null || true
./bin/maestro container rm smoke-nginx 2>/dev/null || true

# 1. Environment Check
echo -e "${YELLOW}[1/6] Checking Environment...${NC}"
./bin/maestro version
./bin/maestro system check
echo -e "${GREEN}Environment OK${NC}"

# 2. Image Management
echo -e "${YELLOW}[2/6] Pulling Nginx Alpine...${NC}"
./bin/maestro pull nginx:alpine
./bin/maestro image ls | grep nginx
echo -e "${GREEN}Image Pulled OK${NC}"

# 3. Container Execution (Background with Port Mapping)
echo -e "${YELLOW}[3/6] Starting Nginx Container (Port 8080:80)...${NC}"
# Use a random or free port if 8080 is common, but the plan says 8080.
./bin/maestro run -d -p 8080:80 --name smoke-nginx nginx:alpine
echo -e "${GREEN}Container Started${NC}"

# 4. Networking Verification
echo -e "${YELLOW}[4/6] Verifying Connectivity (curl localhost:8080)...${NC}"
# Wait a few seconds for Nginx and pasta to be ready
echo "Waiting for Nginx to initialize..."
sleep 5

if curl -s --retry 5 --retry-delay 2 localhost:8080 | grep "Welcome to nginx!" > /dev/null; then
    echo -e "${GREEN}Connectivity OK: Nginx is reachable at localhost:8080${NC}"
else
    echo -e "${RED}Error: Failed to reach Nginx at localhost:8080${NC}"
    echo "Container Logs:"
    ./bin/maestro container logs smoke-nginx
    echo "Maestro System Info:"
    ./bin/maestro system info
    echo "Cleaning up before exit..."
    ./bin/maestro container stop smoke-nginx || true
    ./bin/maestro container rm smoke-nginx || true
    exit 1
fi

# 5. Logs Verification
echo -e "${YELLOW}[5/6] Checking Logs...${NC}"
./bin/maestro container logs smoke-nginx | tail -n 10
echo -e "${GREEN}Logs OK${NC}"

# 6. Cleanup
echo -e "${YELLOW}[6/6] Cleaning Up...${NC}"
./bin/maestro container stop smoke-nginx || true
./bin/maestro container rm smoke-nginx || true
echo -e "${GREEN}Cleanup OK${NC}"

echo -e "${GREEN}==> Maestro Nginx Smoke Test Successfully Completed!${NC}"
