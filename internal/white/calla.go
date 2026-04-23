package white

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

const minSubIDRange = 65536

// IDMapping represents a UID/GID mapping for a user namespace.
type IDMapping struct {
	ContainerID uint32
	HostID      uint32
	Size        uint32
}

// GetSubIDRange reads the subordinate ID range for a user from a file (e.g., /etc/subuid).
func GetSubIDRange(username string, path string) (uint32, uint32, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		const expectedIDParts = 3
		if len(parts) != expectedIDParts {
			continue
		}
		if parts[0] == username {
			startVal, errParseStart := strconv.ParseUint(parts[1], 10, 32)
			if errParseStart != nil {
				return 0, 0, fmt.Errorf(
					"failed to parse start of subordinate ID range in %s: %w",
					path,
					errParseStart,
				)
			}
			countVal, errParseCount := strconv.ParseUint(parts[2], 10, 32)
			if errParseCount != nil {
				return 0, 0, fmt.Errorf(
					"failed to parse count of subordinate ID range in %s: %w",
					path,
					errParseCount,
				)
			}
			return uint32(startVal), uint32(countVal), nil
		}
	}

	// coverage:ignore reachable only on IO failure during scan
	if errScan := scanner.Err(); errScan != nil {
		return 0, 0, fmt.Errorf("failed to scan %s: %w", path, errScan)
	}

	return 0, 0, fmt.Errorf("no subordinate ID allocation found for user %s in %s", username, path)
}

// ValidateSubIDRange checks if the subordinate ID range is sufficient (at least 65536).
func ValidateSubIDRange(count uint32) error {
	if count < minSubIDRange {
		return fmt.Errorf(
			"insufficient subordinate ID range (found %d, required %d)",
			count,
			minSubIDRange,
		)
	}
	return nil
}

// BuildIDMappings generates the UID and GID mapping sets for a rootless container using default paths.
func BuildIDMappings(username string, hostUID, hostGID uint32) ([]IDMapping, []IDMapping, error) {
	return buildMappings(username, hostUID, hostGID, "/etc/subuid", "/etc/subgid")
}

func buildMappings(
	username string,
	hostUID, hostGID uint32,
	subuidPath, subgidPath string,
) ([]IDMapping, []IDMapping, error) {
	subUIDStart, subUIDCount, err := GetSubIDRange(username, subuidPath)
	if err != nil {
		return nil, nil, err
	}
	if errValUID := ValidateSubIDRange(subUIDCount); errValUID != nil {
		return nil, nil, errValUID
	}

	subGIDStart, subGIDCount, errGID := GetSubIDRange(username, subgidPath)
	if errGID != nil {
		return nil, nil, errGID
	}
	if errValGID := ValidateSubIDRange(subGIDCount); errValGID != nil {
		return nil, nil, errValGID
	}

	// 1. Map container ID 0 to host UID/GID
	// 2. Map container IDs 1..N to subordinate ranges
	uidMappings := []IDMapping{
		{ContainerID: 0, HostID: hostUID, Size: 1},
		{ContainerID: 1, HostID: subUIDStart, Size: subUIDCount},
	}
	gidMappings := []IDMapping{
		{ContainerID: 0, HostID: hostGID, Size: 1},
		{ContainerID: 1, HostID: subGIDStart, Size: subGIDCount},
	}

	return uidMappings, gidMappings, nil
}

// ApplyIDMappings invokes newuidmap and newgidmap to apply the given mappings
// to the target PID. This is required for rootless subordinate ID mappings.
//
// We intentionally do NOT write "deny" to /proc/PID/setgroups before calling
// newgidmap. Writing "deny" is only required when gid_map is written by an
// unprivileged process without CAP_SETGID in the parent user namespace.
// newgidmap is a setuid-root binary that has CAP_SETGID in init_user_ns, so
// the kernel permits it to write gid_map unconditionally. Keeping the holder's
// user namespace in the default setgroups=allow state allows container
// processes (e.g. nginx dropping to uid=101) to call initgroups/setgroups.
func ApplyIDMappings(pid int, uidMaps, gidMap []IDMapping) error {
	if err := applyMapping("newuidmap", pid, uidMaps); err != nil {
		return err
	}
	return applyMapping("newgidmap", pid, gidMap)
}

func applyMapping(tool string, pid int, ms []IDMapping) error {
	args := []string{strconv.Itoa(pid)}
	for _, m := range ms {
		args = append(args,
			strconv.FormatUint(uint64(m.ContainerID), 10),
			strconv.FormatUint(uint64(m.HostID), 10),
			strconv.FormatUint(uint64(m.Size), 10),
		)
	}

	log.Debug().Str("tool", tool).Int("pid", pid).Strs("args", args).Msg("applying ID mapping")

	cmd := exec.CommandContext(context.Background(), tool, args...)
	if out, errRun := cmd.CombinedOutput(); errRun != nil {
		return fmt.Errorf(
			"%s failed (pid: %d, args: %v): %w (output: %s)",
			tool,
			pid,
			args,
			errRun,
			string(out),
		)
	}
	return nil
}

// CurrentUser returns the current user's username.
func CurrentUser() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// CurrentIDs returns the numeric UID and GID of the current user.
func CurrentIDs() (uint32, uint32, error) {
	u, err := user.Current()
	if err != nil {
		return 0, 0, err
	}
	uid, errUID := strconv.ParseUint(u.Uid, 10, 32)
	if errUID != nil {
		return 0, 0, fmt.Errorf("failed to parse current UID %q: %w", u.Uid, errUID)
	}
	gid, errGID := strconv.ParseUint(u.Gid, 10, 32)
	if errGID != nil {
		return 0, 0, fmt.Errorf("failed to parse current GID %q: %w", u.Gid, errGID)
	}
	return uint32(uid), uint32(gid), nil
}
