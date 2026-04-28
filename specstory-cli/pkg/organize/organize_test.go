package organize

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRepoRoots(t *testing.T) {
	tmpDir := t.TempDir()
	repo1 := filepath.Join(tmpDir, "repo-one")
	history1 := filepath.Join(repo1, SpecStoryDir, HistoryDir)
	if err := os.MkdirAll(history1, 0755); err != nil {
		t.Fatalf("failed to create history directory: %v", err)
	}

	repo2 := filepath.Join(tmpDir, "repo-two")
	history2 := filepath.Join(repo2, SpecStoryDir, HistoryDir)
	if err := os.MkdirAll(history2, 0755); err != nil {
		t.Fatalf("failed to create history directory: %v", err)
	}

	roots, err := DiscoverRepoRoots(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverRepoRoots returned error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
}

func TestResolveRootPathsAndDiscoverHistoryFiles(t *testing.T) {
	tmpDir := t.TempDir()
	repo := filepath.Join(tmpDir, "repo")
	historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		t.Fatalf("failed to create history directory: %v", err)
	}

	filePath := filepath.Join(historyDir, "2026-01-01_test.md")
	if err := os.WriteFile(filePath, []byte("# session\n"), 0644); err != nil {
		t.Fatalf("failed to write history file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repo})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	files, err := DiscoverHistoryFiles(roots)
	if err != nil {
		t.Fatalf("DiscoverHistoryFiles returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != filePath {
		t.Fatalf("expected file path %s, got %s", filePath, files[0].Path)
	}
}

func TestEnsureHistoryDirectoriesCreatesMissing(t *testing.T) {
	tmpDir := t.TempDir()
	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatalf("failed to create repo root: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repo})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	created, err := EnsureHistoryDirectories(roots)
	if err != nil {
		t.Fatalf("EnsureHistoryDirectories returned error: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created root, got %d", len(created))
	}

	historyPath := filepath.Join(repo, SpecStoryDir, HistoryDir)
	if info, err := os.Stat(historyPath); err != nil {
		t.Fatalf("history directory missing after EnsureHistoryDirectories: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected history path to be a directory")
	}
}

func TestBuildRelocationPlanIdentifiesMoveAndInPlaceEntries(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	inPlaceFile := filepath.Join(repoB, SpecStoryDir, HistoryDir, "session-b.md")
	if err := os.WriteFile(inPlaceFile, []byte("This session belongs to repo-b."), 0644); err != nil {
		t.Fatalf("failed to write in-place history file: %v", err)
	}

	moveFile := filepath.Join(repoA, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(moveFile, []byte("This session refers to repo-b at "+repoB+" and repo-b."), 0644); err != nil {
		t.Fatalf("failed to write move history file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repoA, repoB})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	plan, err := BuildRelocationPlan(roots)
	if err != nil {
		t.Fatalf("BuildRelocationPlan returned error: %v", err)
	}

	if len(plan.Files) != 2 {
		t.Fatalf("expected 2 history files, got %d", len(plan.Files))
	}
	if len(plan.InPlace) != 1 {
		t.Fatalf("expected 1 in-place entry, got %d", len(plan.InPlace))
	}
	if len(plan.Moves) != 1 {
		t.Fatalf("expected 1 move entry, got %d", len(plan.Moves))
	}

	moveEntry := plan.Moves[0]
	if moveEntry.File.Path != moveFile {
		t.Fatalf("expected move entry source %q, got %q", moveFile, moveEntry.File.Path)
	}
	if moveEntry.TargetRepo == nil || moveEntry.TargetRepo.Path != repoB {
		t.Fatalf("expected move entry target repo %q, got %v", repoB, moveEntry.TargetRepo)
	}
	if moveEntry.TargetPath != filepath.Join(repoB, SpecStoryDir, HistoryDir, filepath.Base(moveFile)) {
		t.Fatalf("unexpected move target path: %q", moveEntry.TargetPath)
	}
}

func TestBuildRelocationPlanMarksAmbiguousWhenNoRepoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	ambiguousFile := filepath.Join(repoA, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(ambiguousFile, []byte("No repository name or path appears here."), 0644); err != nil {
		t.Fatalf("failed to write ambiguous history file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repoA, repoB})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	plan, err := BuildRelocationPlan(roots)
	if err != nil {
		t.Fatalf("BuildRelocationPlan returned error: %v", err)
	}
	if len(plan.Ambiguous) != 1 {
		t.Fatalf("expected 1 ambiguous entry, got %d", len(plan.Ambiguous))
	}
	if plan.Ambiguous[0].File.Path != ambiguousFile {
		t.Fatalf("expected ambiguous file %q, got %q", ambiguousFile, plan.Ambiguous[0].File.Path)
	}
}

func TestApplyRelocationPlanResolvesExistingDestFile(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	moveFile := filepath.Join(repoA, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(moveFile, []byte("Move this to repo-b at "+repoB), 0644); err != nil {
		t.Fatalf("failed to write move history file: %v", err)
	}

	existingDest := filepath.Join(repoB, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(existingDest, []byte("existing session"), 0644); err != nil {
		t.Fatalf("failed to write existing destination file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repoA, repoB})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	plan, err := BuildRelocationPlan(roots)
	if err != nil {
		t.Fatalf("BuildRelocationPlan returned error: %v", err)
	}

	moved, err := ApplyRelocationPlan(plan)
	if err != nil {
		t.Fatalf("ApplyRelocationPlan returned error: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 moved file, got %d", moved)
	}

	// Existing destination should remain and moved file should be renamed with numeric suffix.
	if _, err := os.Stat(existingDest); err != nil {
		t.Fatalf("expected original destination file to remain: %v", err)
	}

	movedTarget := existingDest + ".1"
	if _, err := os.Stat(movedTarget); err != nil {
		t.Fatalf("expected moved file to resolve collision at %s: %v", movedTarget, err)
	}
	if _, err := os.Stat(moveFile); err == nil {
		t.Fatalf("expected source file to be removed after move")
	}
}

func TestFindAbsolutePathsHandlesWindowsAndUnixPaths(t *testing.T) {
	input := "C:\\repo-b\\.specstory\\history\\session.md /repo-a/.specstory/history/session.md \\\\server\\share\\repo\\.specstory\\history"
	paths := findAbsolutePaths(input)
	if len(paths) != 3 {
		t.Fatalf("expected 3 absolute paths, got %d: %v", len(paths), paths)
	}

	expected := map[string]bool{
		"C:\\repo-b\\.specstory\\history\\session.md":  true,
		"/repo-a/.specstory/history/session.md":        true,
		"\\\\server\\share\\repo\\.specstory\\history": true,
	}
	for _, path := range paths {
		if !expected[path] {
			t.Fatalf("unexpected path match: %s", path)
		}
	}
}

func TestBuildRelocationPlanDisambiguatesSessionFiles(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	fileA := filepath.Join(repoA, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(fileA, []byte("This session refers to repo-b and a path /repo-b/some-file"), 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repoA, repoB})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	plan, err := BuildRelocationPlan(roots)
	if err != nil {
		t.Fatalf("BuildRelocationPlan returned error: %v", err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(plan.Files))
	}
	if len(plan.Moves) != 1 {
		t.Fatalf("expected 1 move, got %d", len(plan.Moves))
	}
	if plan.Moves[0].TargetRepo == nil || plan.Moves[0].TargetRepo.Path != repoB {
		t.Fatalf("expected target repo %q, got %v", repoB, plan.Moves[0].TargetRepo)
	}
}

func TestApplyRelocationPlanMovesFile(t *testing.T) {
	tmpDir := t.TempDir()
	repoA := filepath.Join(tmpDir, "repo-a")
	repoB := filepath.Join(tmpDir, "repo-b")
	for _, repo := range []string{repoA, repoB} {
		historyDir := filepath.Join(repo, SpecStoryDir, HistoryDir)
		if err := os.MkdirAll(historyDir, 0755); err != nil {
			t.Fatalf("failed to create history directory: %v", err)
		}
	}

	fileA := filepath.Join(repoA, SpecStoryDir, HistoryDir, "session-a.md")
	if err := os.WriteFile(fileA, []byte("This session refers to repo-b and a path /repo-b/some-file"), 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}

	roots, err := ResolveRootPaths([]string{repoA, repoB})
	if err != nil {
		t.Fatalf("ResolveRootPaths returned error: %v", err)
	}

	plan, err := BuildRelocationPlan(roots)
	if err != nil {
		t.Fatalf("BuildRelocationPlan returned error: %v", err)
	}

	moved, err := ApplyRelocationPlan(plan)
	if err != nil {
		t.Fatalf("ApplyRelocationPlan returned error: %v", err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 moved file, got %d", moved)
	}

	targetPath := filepath.Join(repoB, SpecStoryDir, HistoryDir, filepath.Base(fileA))
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("expected moved file at %s: %v", targetPath, err)
	}
	if _, err := os.Stat(fileA); err == nil {
		t.Fatalf("expected source file to be removed after move")
	}
}
