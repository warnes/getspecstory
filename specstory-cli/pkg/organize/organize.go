package organize

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SpecStoryDir  = ".specstory"
	HistoryDir    = "history"
	SessionSuffix = ".md"
)

// RepoRoot is a discovered repository root that contains or may contain SpecStory history.
type RepoRoot struct {
	Alias string
	Path  string
}

// HistoryFile represents a discovered session markdown file.
type HistoryFile struct {
	RepoAlias string
	Path      string
}

type PlanEntry struct {
	File       HistoryFile
	SourceRepo *RepoRoot
	TargetRepo *RepoRoot
	TargetPath string
	Reason     string
}

type RelocationPlan struct {
	Roots     []RepoRoot
	Files     []HistoryFile
	InPlace   []PlanEntry
	Moves     []PlanEntry
	Ambiguous []PlanEntry
}

func (p RelocationPlan) Total() int {
	return len(p.InPlace) + len(p.Moves) + len(p.Ambiguous)
}

// DiscoverRepoRoots searches baseDir recursively for directories containing .specstory/history.
func DiscoverRepoRoots(baseDir string) ([]RepoRoot, error) {
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base directory %q: %w", baseDir, err)
	}

	rootMap := make(map[string]RepoRoot)

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() != SpecStoryDir {
			return nil
		}

		historyPath := filepath.Join(path, HistoryDir)
		info, err := os.Stat(historyPath)
		if err != nil || !info.IsDir() {
			return filepath.SkipDir
		}

		repoRoot := filepath.Dir(path)
		repoRoot, err = filepath.Abs(repoRoot)
		if err != nil {
			return err
		}

		alias := filepath.Base(repoRoot)
		alias = uniqueAlias(alias, rootMap)
		rootMap[repoRoot] = RepoRoot{Alias: alias, Path: repoRoot}
		return filepath.SkipDir
	}

	if err := filepath.WalkDir(baseDir, walkFn); err != nil {
		return nil, err
	}

	roots := make([]RepoRoot, 0, len(rootMap))
	for _, root := range rootMap {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Alias < roots[j].Alias
	})

	return roots, nil
}

// ResolveRootPaths resolves explicit root paths and validates that they exist.
func ResolveRootPaths(roots []string) ([]RepoRoot, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("no repository roots provided")
	}

	rootMap := make(map[string]RepoRoot)
	for _, raw := range roots {
		resolved, err := expandTilde(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid root %q: %w", raw, err)
		}

		absRoot, err := filepath.Abs(resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve root %q: %w", raw, err)
		}

		info, err := os.Stat(absRoot)
		if err != nil {
			return nil, fmt.Errorf("root path does not exist: %s", absRoot)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("root path is not a directory: %s", absRoot)
		}

		alias := filepath.Base(absRoot)
		alias = uniqueAlias(alias, rootMap)
		rootMap[absRoot] = RepoRoot{Alias: alias, Path: absRoot}
	}

	result := make([]RepoRoot, 0, len(rootMap))
	for _, root := range rootMap {
		result = append(result, root)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Alias < result[j].Alias
	})

	return result, nil
}

// DiscoverHistoryFiles locates markdown session files under each repo history directory.
func DiscoverHistoryFiles(repoRoots []RepoRoot) ([]HistoryFile, error) {
	files := make([]HistoryFile, 0)
	for _, root := range repoRoots {
		historyPath := filepath.Join(root.Path, SpecStoryDir, HistoryDir)
		entries, err := os.ReadDir(historyPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read history directory %s: %w", historyPath, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.EqualFold(filepath.Ext(name), SessionSuffix) {
				files = append(files, HistoryFile{
					RepoAlias: root.Alias,
					Path:      filepath.Join(historyPath, name),
				})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].RepoAlias != files[j].RepoAlias {
			return files[i].RepoAlias < files[j].RepoAlias
		}
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// EnsureHistoryDirectories creates missing .specstory/history directories for explicit roots.
func EnsureHistoryDirectories(repoRoots []RepoRoot) ([]RepoRoot, error) {
	created := make([]RepoRoot, 0)
	for _, root := range repoRoots {
		historyPath := filepath.Join(root.Path, SpecStoryDir, HistoryDir)
		_, err := os.Stat(historyPath)
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to inspect history path %s: %w", historyPath, err)
		}
		if err := os.MkdirAll(historyPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create history directory %s: %w", historyPath, err)
		}
		created = append(created, root)
	}
	return created, nil
}

// BuildRelocationPlan discovers history files and builds a relocation plan.
func BuildRelocationPlan(repoRoots []RepoRoot) (RelocationPlan, error) {
	files, err := DiscoverHistoryFiles(repoRoots)
	if err != nil {
		return RelocationPlan{}, err
	}

	plan := RelocationPlan{
		Roots: repoRoots,
		Files: files,
	}

	for _, file := range files {
		sourceRoot := findRootByAlias(file.RepoAlias, repoRoots)
		entry := classifyHistoryFile(file, repoRoots, sourceRoot)
		entry.SourceRepo = sourceRoot
		if entry.TargetRepo == nil {
			plan.Ambiguous = append(plan.Ambiguous, entry)
			continue
		}
		if sourceRoot != nil && entry.TargetRepo.Path == sourceRoot.Path {
			plan.InPlace = append(plan.InPlace, entry)
			continue
		}
		plan.Moves = append(plan.Moves, entry)
	}

	return plan, nil
}

// PrintRelocationPlan prints a human-readable relocation plan.
func PrintRelocationPlan(plan RelocationPlan, verbose bool) {
	fmt.Printf("Discovered %d repository root(s) and %d session file(s)\n", len(plan.Roots), len(plan.Files))
	fmt.Printf("  in place: %d, move: %d, ambiguous: %d\n", len(plan.InPlace), len(plan.Moves), len(plan.Ambiguous))

	if len(plan.Moves) > 0 {
		fmt.Println()
		fmt.Println("Planned relocations:")
		for _, entry := range plan.Moves {
			fmt.Printf("- %s -> %s (%s)\n", entry.File.Path, entry.TargetPath, entry.Reason)
		}
	}

	if len(plan.Ambiguous) > 0 {
		fmt.Println()
		fmt.Println("Ambiguous files:")
		for _, entry := range plan.Ambiguous {
			fmt.Printf("- %s (%s)\n", entry.File.Path, entry.Reason)
		}
	}

	if verbose {
		fmt.Println()
		fmt.Println("In-place files:")
		for _, entry := range plan.InPlace {
			fmt.Printf("- %s\n", entry.File.Path)
		}
		if len(plan.Moves) > 0 {
			fmt.Println()
			fmt.Println("Session files planned for move:")
			for _, entry := range plan.Moves {
				fmt.Printf("- %s -> %s\n", entry.File.Path, entry.TargetPath)
			}
		}
		if len(plan.Ambiguous) > 0 {
			fmt.Println()
			fmt.Println("Session files with ambiguous targets:")
			for _, entry := range plan.Ambiguous {
				fmt.Printf("- %s\n", entry.File.Path)
			}
		}
	}
}

// ApplyRelocationPlan moves files from source history locations into their target repo history directories.
func ApplyRelocationPlan(plan RelocationPlan) (int, error) {
	moved := 0
	for _, entry := range plan.Moves {
		targetDir := filepath.Dir(entry.TargetPath)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return moved, fmt.Errorf("failed to create target history directory %s: %w", targetDir, err)
		}

		dest := entry.TargetPath
		if exists(dest) {
			dest = resolveCollision(dest)
		}

		if err := renameOrCopy(entry.File.Path, dest); err != nil {
			return moved, fmt.Errorf("failed to relocate %s to %s: %w", entry.File.Path, dest, err)
		}
		moved++
	}
	return moved, nil
}

func classifyHistoryFile(file HistoryFile, repoRoots []RepoRoot, sourceRoot *RepoRoot) PlanEntry {
	text, err := os.ReadFile(file.Path)
	reason := "ambiguous target"
	if err != nil {
		reason = fmt.Sprintf("failed to read file: %v", err)
		return PlanEntry{File: file, TargetRepo: nil, TargetPath: "", Reason: reason}
	}

	target := findTargetRepoForHistoryFile(string(text), repoRoots)
	entry := PlanEntry{File: file, TargetRepo: target}
	if target == nil {
		entry.Reason = reason
		return entry
	}
	if sourceRoot != nil && target.Path == sourceRoot.Path {
		entry.TargetPath = file.Path
		entry.Reason = "already in correct repo"
		return entry
	}
	entry.TargetPath = filepath.Join(target.Path, SpecStoryDir, HistoryDir, filepath.Base(file.Path))
	entry.Reason = fmt.Sprintf("matched target repo %q", target.Alias)
	return entry
}

func findTargetRepoForHistoryFile(text string, repoRoots []RepoRoot) *RepoRoot {
	weights := make(map[int]int)
	bestWeight := 0
	var best *RepoRoot

	for i := range repoRoots {
		r := &repoRoots[i]
		weight := 0
		weight += scorePaths(text, r)
		weight += scoreKeywords(text, r)

		weights[i] = weight
		if weight > bestWeight {
			bestWeight = weight
			best = r
		} else if weight == bestWeight {
			best = nil
		}
	}

	if bestWeight == 0 {
		return nil
	}
	return best
}

func scorePaths(text string, root *RepoRoot) int {
	count := 0
	for _, path := range findAbsolutePaths(text) {
		if rootMatch(path, root) {
			count += 20
		}
	}
	return count
}

func scoreKeywords(text string, root *RepoRoot) int {
	weight := 0
	aliasPattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(root.Alias) + `\b`)
	if aliasPattern.MatchString(text) {
		weight += 5
	}
	baseName := filepath.Base(root.Path)
	if baseName != root.Alias {
		basePattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(baseName) + `\b`)
		if basePattern.MatchString(text) {
			weight += 3
		}
	}
	return weight
}

func findAbsolutePaths(text string) []string {
	pathRe := regexp.MustCompile(`(?:[A-Za-z]:\\|\\\\|/)[\w\-.\\/]+`)
	matches := pathRe.FindAllString(text, -1)
	unique := make(map[string]struct{})
	results := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, ok := unique[match]; ok {
			continue
		}
		unique[match] = struct{}{}
		results = append(results, match)
	}
	return results
}

func normalizePath(path string) string {
	normalized := strings.ReplaceAll(path, `/`, string(os.PathSeparator))
	normalized = strings.ReplaceAll(normalized, `\\`, string(os.PathSeparator))
	return filepath.Clean(normalized)
}

func rootMatch(path string, root *RepoRoot) bool {
	cleaned := normalizePath(path)
	rootPath := normalizePath(root.Path)
	if cleaned == rootPath {
		return true
	}
	return strings.HasPrefix(cleaned, rootPath+string(os.PathSeparator))
}

func findRootByAlias(alias string, repoRoots []RepoRoot) *RepoRoot {
	for i := range repoRoots {
		if repoRoots[i].Alias == alias {
			return &repoRoots[i]
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolveCollision(path string) string {
	base := filepath.Base(path)
	dir := filepath.Dir(path)
	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s.%d", base, i))
		if !exists(candidate) {
			return candidate
		}
	}
}

func renameOrCopy(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, input, 0644); err != nil {
		return err
	}
	return os.Remove(src)
}

func uniqueAlias(base string, existing map[string]RepoRoot) string {
	alias := base
	suffix := 1
	for {
		collision := false
		for _, root := range existing {
			if root.Alias == alias {
				collision = true
				break
			}
		}
		if !collision {
			return alias
		}
		alias = fmt.Sprintf("%s_%d", base, suffix)
		suffix++
	}
}

func expandTilde(path string) (string, error) {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
