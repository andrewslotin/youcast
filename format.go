package main

import "strconv"

// FileSize is a file size in bytes.
type FileSize int64

// String returns a human-readable representation of the file size.
func (fs FileSize) String() string {
	switch {
	case fs > 2<<39: // TB
		return strconv.FormatFloat(float64(fs)/float64(2<<39), 'f', 2, 64) + " TB"
	case fs > 2<<29: // GB
		return strconv.FormatFloat(float64(fs)/float64(2<<29), 'f', 2, 64) + " GB"
	case fs > 2<<19: // MB
		return strconv.FormatFloat(float64(fs)/float64(2<<19), 'f', 2, 64) + " MB"
	case fs > 2<<9: // kB
		return strconv.FormatInt(int64(fs)/2<<9, 10) + " kB"
	case fs == 1:
		return "1 byte"
	default:
		return strconv.FormatInt(int64(fs), 10) + "bytes"
	}
}
