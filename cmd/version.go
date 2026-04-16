package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Defaults. The release build overrides these via ldflags; for `go install`
// and plain `go build`, fillFromBuildInfo() populates them from the module
// metadata Go embeds in every binary since 1.18.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("openpraxis %s (commit: %s, built: %s)\n", Version, GitCommit, BuildDate)
	},
}

// fillFromBuildInfo replaces any default value with data from
// runtime/debug.ReadBuildInfo. ldflags-injected values are left alone because
// they no longer equal their defaults.
func fillFromBuildInfo() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if GitCommit == "unknown" && s.Value != "" {
				commit := s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
				GitCommit = commit
			}
		case "vcs.time":
			if BuildDate == "unknown" && s.Value != "" {
				BuildDate = s.Value
			}
		}
	}
}

func init() {
	fillFromBuildInfo()
	rootCmd.AddCommand(versionCmd)
}
