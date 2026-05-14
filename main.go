package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/hellowynd/wyndctl/cmd"
)

// These are populated at build time via -ldflags. See Makefile.
var (
	version    = "dev"
	buildTime  = "" // RFC3339, UTC
	gitSHA     = "unknown"
	gitDirty   = "clean"
	maxAgeDays = "0" // 0 disables the kill-date check
)

func main() {
	if err := checkBinaryAge(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	cmd.BuildInfo = cmd.BuildMetadata{
		Version:   version,
		BuildTime: buildTime,
		GitSHA:    gitSHA,
		GitDirty:  gitDirty == "dirty",
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func checkBinaryAge() error {
	if buildTime == "" {
		return nil
	}
	maxDays, err := strconv.Atoi(maxAgeDays)
	if err != nil || maxDays <= 0 {
		return nil
	}
	t, err := time.Parse(time.RFC3339, buildTime)
	if err != nil {
		return nil
	}
	age := time.Since(t)
	maxAge := time.Duration(maxDays) * 24 * time.Hour
	if age > maxAge {
		return fmt.Errorf(
			"this wyndctl binary was built %s (%.0f days ago) and exceeds the %d-day max age.\n"+
				"Rebuild with `make install` to continue. (built from %s, version %s)",
			t.Format("2006-01-02"), age.Hours()/24, maxDays, gitSHA, version)
	}
	return nil
}
