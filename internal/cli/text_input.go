package cli

import (
	"fmt"
	"io"
	"os"
)

// readTextArg resolves a description/comment text argument shared by
// issue create/edit/comment: "-" reads stdin, an existing file path reads
// that file's contents, and anything else is used literally. Empty input
// returns "".
func readTextArg(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if s == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(b), nil
	}
	if info, err := os.Stat(s); err == nil && !info.IsDir() {
		b, err := os.ReadFile(s)
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", s, err)
		}
		return string(b), nil
	}
	return s, nil
}
