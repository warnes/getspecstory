package cmd

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/specstoryai/getspecstory/specstory-cli/pkg/organize"
)

// organizeFlags holds the flags for the organize command.
var organizeFlags struct {
	roots   []string
	baseDir string
	apply   bool
	verbose bool
}

// CreateOrganizeCommand creates the organize command.
func CreateOrganizeCommand() *cobra.Command {
	examples := `
# Discover repository roots and session histories under the current directory
specstory organize

# Discover roots under a specific directory
specstory organize --base-dir ~/src

# Explicitly scan specific repositories
specstory organize --roots ~/src/repo1 ~/src/repo2

# Apply the inferred relocation plan and move misplaced session history files
specstory organize --roots ~/src/repo1 ~/src/repo2 --apply`

	longDesc := `Discover repository roots by locating .specstory/history folders, infer the correct repository for session history files, and optionally relocate misplaced history to the matching repo.`

	cmd := &cobra.Command{
		Use:     "organize",
		Short:   "Discover and organize SpecStory history across repositories",
		Long:    longDesc,
		Example: examples,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Info("Running organize command")

			var roots []organize.RepoRoot
			var err error
			if len(organizeFlags.roots) > 0 {
				roots, err = organize.ResolveRootPaths(organizeFlags.roots)
			} else {
				roots, err = organize.DiscoverRepoRoots(organizeFlags.baseDir)
			}
			if err != nil {
				return err
			}
			if len(roots) == 0 {
				return fmt.Errorf("no repository roots discovered. Provide --roots or run from a directory containing .specstory/history folders")
			}

			plan, err := organize.BuildRelocationPlan(roots)
			if err != nil {
				return err
			}

			organize.PrintRelocationPlan(plan, organizeFlags.verbose)

			if organizeFlags.apply {
				moved, err := organize.ApplyRelocationPlan(plan)
				if err != nil {
					return err
				}
				fmt.Println()
				if moved > 0 {
					fmt.Printf("Moved %d session file(s) to their inferred repositories.\n", moved)
				} else {
					fmt.Println("No session files were moved.")
				}
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&organizeFlags.roots, "roots", []string{}, "Explicit repository root directories to scan")
	cmd.Flags().StringVar(&organizeFlags.baseDir, "base-dir", ".", "Base directory to discover repository roots when --roots is omitted")
	cmd.Flags().BoolVar(&organizeFlags.apply, "apply", false, "Apply the inferred relocation plan and move misplaced session history files")
	cmd.Flags().BoolVar(&organizeFlags.verbose, "verbose", false, "Show detailed session file discovery output")

	return cmd
}
