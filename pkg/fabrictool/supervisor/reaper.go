/*
Copyright 2025 The Scion Authors.
*/

package supervisor

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/GoogleCloudPlatform/scion/pkg/sciontool/log"
)

// snapshotProcessNames reads process names from /proc for all current
// child processes. This must be called before reaping, since /proc/<pid>
// entries are removed once wait() completes.
func snapshotProcessNames() map[int]string {
	names := make(map[int]string)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return names
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(string(comm)); name != "" {
			names[pid] = name
		}
	}
	return names
}

// StartReaper starts a goroutine that reaps zombie processes and logs them.
func StartReaper() {
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGCHLD)

		for range sigs {
			// Snapshot process names before reaping, since /proc/<pid>
			// entries are removed once wait() completes.
			names := snapshotProcessNames()

			for {
				var ws syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if err != nil {
					break
				}
				if pid <= 0 {
					break
				}

				reason := "exited"
				if ws.Signaled() {
					reason = "killed by signal " + ws.Signal().String()
				}
				name := names[pid]
				if name == "" {
					name = "unknown"
				}
				log.Info("Reaped zombie process %d (%s) (reason: %s, exit code: %d)", pid, name, reason, ws.ExitStatus())
			}
		}
	}()
}
