package main

import (
	"os"
	"path/filepath"
)

func (r *runner) uploadBinary() error {
	binaryPath := os.Getenv("SHIP_BINARY")
	if binaryPath == "" {
		binaryPath = filepath.Join(artifacts, "app-linux.test")
	}
	binary, err := os.Open(binaryPath)
	if err != nil {
		return err
	}
	defer binary.Close()
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = binary
	return session.Run("umask 077; cat > " + remoteBinary + "; chmod 700 " + remoteBinary)
}
