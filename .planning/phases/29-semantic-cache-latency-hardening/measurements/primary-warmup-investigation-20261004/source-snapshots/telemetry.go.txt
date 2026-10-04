package main

import "os"

func (r *runner) captureTelemetry(test, phase string) error {
	if os.Getenv("SHIP_TELEMETRY") != "true" {
		return nil
	}
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	output, err := session.CombinedOutput("date -u '+%Y-%m-%dT%H:%M:%SZ' && cat /proc/stat /proc/loadavg /proc/uptime")
	if saveErr := r.save(test+"-vm-"+phase+".log", output); saveErr != nil {
		return saveErr
	}
	return err
}
