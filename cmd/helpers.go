package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirmAction prompts the user for Y/n confirmation.
// Returns true only for explicit "y" or "yes" input.
func confirmAction(message string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "%s [y/N] ", message)

	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
