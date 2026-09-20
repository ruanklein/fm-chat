package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const accountImageFile = "accountimage.jpg"

func captureAccountImage(ctx context.Context, homeDirectory, destination string) error {
	commandContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	output, err := exec.CommandContext(commandContext, "dscl", ".", "-read", homeDirectory, "JPEGPhoto").Output()
	if err != nil {
		return fmt.Errorf("read account image: %w", err)
	}

	imageData, err := decodeJPEGPhoto(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create account image directory: %w", err)
	}
	if err := os.WriteFile(destination, imageData, 0o600); err != nil {
		return fmt.Errorf("write account image: %w", err)
	}
	return nil
}

func decodeJPEGPhoto(output []byte) ([]byte, error) {
	const marker = "JPEGPhoto:"

	var encoded strings.Builder
	foundMarker := false
	for _, line := range strings.Split(string(output), "\n") {
		if !foundMarker {
			markerIndex := strings.Index(line, marker)
			if markerIndex < 0 {
				continue
			}
			foundMarker = true
			line = line[markerIndex+len(marker):]
		}
		encoded.WriteString(strings.Join(strings.Fields(line), ""))
	}
	if !foundMarker || encoded.Len() == 0 {
		return nil, errors.New("account image data not found")
	}

	imageData, err := hex.DecodeString(encoded.String())
	if err != nil {
		return nil, fmt.Errorf("decode account image: %w", err)
	}
	if len(imageData) == 0 {
		return nil, errors.New("account image data is empty")
	}
	return imageData, nil
}

func imageDataURL(imageData []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageData)
}
