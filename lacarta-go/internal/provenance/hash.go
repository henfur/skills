package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

func sha256Sum(b []byte) []byte {
	s := sha256.Sum256(b)
	return s[:]
}

// FileSHA256 is a file's sha256 in hex.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func BytesSHA256(b []byte) string { return hex.EncodeToString(sha256Sum(b)) }
