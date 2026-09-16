package imaging

import (
	"crypto/md5" //nolint:gosec // G501: MD5 is the dedup key the Python history.db stores (R5.5), not a security primitive.
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

// sha256BlockSize is the read size plugins/blacklist.py used; the digest does
// not depend on it, but the port keeps the same I/O pattern (R3.7, R5.6).
const sha256BlockSize = 4096

// MD5File returns the lower-case hex MD5 of the whole file, the value
// plugins/history.py stored as image_hash (R5.5).
func MD5File(path string) (string, error) {
	return hashFile(path, md5.New(), 0) //nolint:gosec // G401: see package import note; parity with history.db.
}

// SHA256File returns the lower-case hex SHA-256 of the file, streamed in
// 4096-byte blocks the way plugins/blacklist.py did (R5.6).
func SHA256File(path string) (string, error) {
	return hashFile(path, sha256.New(), sha256BlockSize)
}

// hashFile feeds path through h. blockSize 0 reads the file in one go.
func hashFile(path string, h hash.Hash, blockSize int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open for hashing: %w", err)
	}
	defer func() { _ = f.Close() }()
	if blockSize <= 0 {
		buf, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		h.Write(buf)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	buf := make([]byte, blockSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
