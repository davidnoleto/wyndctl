package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirmAction prompts the user for Y/n confirmation.
// Returns true only for explicit "y" or "yes" input.
func confirmAction(w io.Writer, r io.Reader, message string) bool {
	fmt.Fprintf(w, "%s [y/N] ", message)

	input, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
