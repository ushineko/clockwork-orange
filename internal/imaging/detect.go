package imaging

import (
	"mime"
	"path/filepath"
	"strings"
)

// ImageExtensions is the extension set clockwork-orange.py accepted (R3.1).
// Compared case-insensitively.
var ImageExtensions = []string{".jpg", ".jpeg", ".png", ".bmp", ".gif", ".tiff", ".webp", ".svg"}

/*
IsImageFile reports whether path looks like an image (R3.1): its extension is
in ImageExtensions, or mime.TypeByExtension maps it to an "image/" type.

Only the name is inspected -- no stat and no decode, as in Python's
is_image_file, whose is_file() pre-check belongs to the caller walking the
directory (the engine already has the entry type in hand).
*/
func IsImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return false
	}
	for _, e := range ImageExtensions {
		if ext == e {
			return true
		}
	}
	return strings.HasPrefix(mime.TypeByExtension(ext), "image/")
}
