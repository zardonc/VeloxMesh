package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const recoveryLimit = 120 * time.Second
const stableSamples = 10
const recoveryBlockKiB = 1024
const recoverySystemCPU = 15
const recoveryWaitCPU = 5

func recoveryStable(line string) (bool, bool) {
	fields := strings.Fields(line)
	if len(fields) < 17 {
		return false, false
	}
	values := make([]int, 17)
	for index := range values {
		value, err := strconv.Atoi(fields[index])
		if err != nil {
			return false, false
		}
		values[index] = value
	}
	return values[6] == 0 && values[7] == 0 && values[8] <= recoveryBlockKiB &&
		values[9] <= recoveryBlockKiB && values[13] < recoverySystemCPU && values[15] < recoveryWaitCPU, true
}

func (r *runner) recoveryGate() error {
	output, err := r.command("cat /proc/meminfo /proc/pressure/memory")
	if err != nil {
		return err
	}
	if err := r.save("recovery-memory.log", output); err != nil {
		return err
	}
	if err := recoveryMemory(output); err != nil {
		return err
	}
	file, err := os.Create(filepath.Join(artifacts, "recovery.jsonl"))
	if err != nil {
		return err
	}
	defer file.Close()
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	stream, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	session.Stderr = os.Stderr
	if err := session.Start("exec timeout 125s vmstat -w 1"); err != nil {
		return err
	}
	timer := time.AfterFunc(recoveryLimit, func() { _ = session.Signal(ssh.SIGTERM); _ = session.Close() })
	defer timer.Stop()
	return observeRecovery(bufio.NewScanner(stream), json.NewEncoder(file))
}

func recoveryMemory(output []byte) error {
	const minimumAvailableKiB = 1024 * 1024
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemAvailable:" {
			continue
		}
		available, err := strconv.Atoi(fields[1])
		if err != nil {
			return err
		}
		if available < minimumAvailableKiB {
			return fmt.Errorf("insufficient MemAvailable at recovery start: %d KiB", available)
		}
		return nil
	}
	return errors.New("MemAvailable missing from recovery evidence")
}

func observeRecovery(scanner *bufio.Scanner, encoder *json.Encoder) error {
	consecutive, rows := 0, 0
	for scanner.Scan() {
		stable, valid := recoveryStable(scanner.Text())
		if !valid {
			continue
		}
		rows++
		if rows == 1 {
			continue
		} // vmstat's boot-average row is not an interval.
		if stable {
			consecutive++
		} else {
			consecutive = 0
		}
		if err := encoder.Encode(map[string]any{"utc": time.Now().UTC(), "line": scanner.Text(),
			"stable": stable, "consecutive": consecutive}); err != nil {
			return err
		}
		if consecutive == stableSamples {
			fmt.Printf("Recovery gate passed: %d consecutive quiet seconds\n", consecutive)
			return nil
		}
	}
	return errors.Join(fmt.Errorf("recovery not achieved within %s", recoveryLimit), scanner.Err())
}
