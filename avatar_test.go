package main

import (
	"bytes"
	"testing"
)

func TestDecodeJPEGPhoto(t *testing.T) {
	output := []byte("JPEGPhoto: ffd8ffe0\n 00104a46 49460001\n ffd9\n")

	imageData, err := decodeJPEGPhoto(output)
	if err != nil {
		t.Fatalf("decodeJPEGPhoto returned an error: %v", err)
	}

	expected := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01, 0xff, 0xd9}
	if !bytes.Equal(imageData, expected) {
		t.Fatalf("decoded image = %x, want %x", imageData, expected)
	}
}

func TestDecodeJPEGPhotoRejectsInvalidOutput(t *testing.T) {
	for _, output := range [][]byte{
		[]byte("No photo here\n"),
		[]byte("JPEGPhoto: abc\n"),
		[]byte("JPEGPhoto: not-hex\n"),
	} {
		if _, err := decodeJPEGPhoto(output); err == nil {
			t.Fatalf("decodeJPEGPhoto(%q) returned no error", output)
		}
	}
}
