package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/specstoryai/getspecstory/specstory-cli/pkg/organize"
)

func TestCreateOrganizeCommandApplyMovesHistoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, organize.SpecStoryDir, organize.HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	inputPath := filepath.Join(repoA, organize.SpecStoryDir, organize.HistoryDir, "session-a.md")
	if err := os.WriteFile(inputPath, []byte("This session refers to repo-b at /repo-b and C:\\repo-b"), 0644); err != nil {
		t.Fatalf("failed to write history file: %v", err)
	}

	cmd := CreateOrganizeCommand()
	cmd.SetArgs([]string{"--roots", repoA + "," + repoB, "--apply"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("organize command returned error: %v", err)
	}

	targetPath := filepath.Join(repoB, organize.SpecStoryDir, organize.HistoryDir, filepath.Base(inputPath))
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected moved file at target path %s: %v", targetPath, err)
	}
	if _, err := os.Stat(inputPath); err == nil {
		t.Fatalf("expected source file to be removed after move")
	}
}
