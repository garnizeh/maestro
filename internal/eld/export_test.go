package eld

import (
	"io"
	"os"
)

// Exported for testing from eld_test package.

type LogLine = logLine

func WriteLogLine(w io.Writer, line LogLine) {
	writeLogLine(w, line)
}

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return atomicWriteFile(path, data, perm)
}
