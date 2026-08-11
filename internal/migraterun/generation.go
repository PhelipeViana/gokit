package migraterun

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/PhelipeViana/gokit/internal/config"
	"github.com/PhelipeViana/gokit/internal/i18n"
)

type generationEntry struct {
	Target    string    `json:"target"`
	Kind      string    `json:"kind"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type generationJournal struct {
	Entries []generationEntry `json:"entries"`
}

type DevelopmentRollbackPlan struct {
	Target  string
	Files   []string
	Blocked []string
}

func generationJournalPath(root string) string {
	return filepath.Join(root, "internal", "gokit", ".state", "generated-files.json")
}

func recordGeneratedFile(root, target, kind, path string) error {
	if err := ensureGenerationStateIgnored(root); err != nil {
		return err
	}
	absRoot, _ := filepath.Abs(root)
	absPath, _ := filepath.Abs(path)
	relative, err := filepath.Rel(absRoot, absPath)
	if err != nil || strings.HasPrefix(relative, "..") {
		return i18n.Errf("gen_outside_project", path)
	}
	journal, _ := loadGenerationJournal(root)
	journal.Entries = append(journal.Entries, generationEntry{
		Target: strings.ToLower(strings.TrimSpace(target)), Kind: kind,
		Path: filepath.ToSlash(relative), CreatedAt: time.Now(),
	})
	return writeGenerationJournal(root, journal)
}

func ensureGenerationStateIgnored(root string) error {
	path := filepath.Join(root, ".gitignore")
	const rule = "internal/gokit/.state/"
	data, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(data), rule) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, []byte(rule+"\n")...)
	return os.WriteFile(path, data, 0o644)
}

func loadGenerationJournal(root string) (generationJournal, error) {
	data, err := os.ReadFile(generationJournalPath(root))
	if os.IsNotExist(err) {
		return generationJournal{}, nil
	}
	if err != nil {
		return generationJournal{}, err
	}
	var journal generationJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return generationJournal{}, err
	}
	return journal, nil
}

func writeGenerationJournal(root string, journal generationJournal) error {
	path := generationJournalPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func PlanDevelopmentRollback(root string) (DevelopmentRollbackPlan, error) {
	journal, err := loadGenerationJournal(root)
	if err != nil {
		return DevelopmentRollbackPlan{}, err
	}
	if len(journal.Entries) == 0 {
		return DevelopmentRollbackPlan{}, i18n.Errf("gen_nothing_registered")
	}
	target := journal.Entries[len(journal.Entries)-1].Target
	plan := DevelopmentRollbackPlan{Target: target}
	seen := map[string]bool{}
	for index := len(journal.Entries) - 1; index >= 0; index-- {
		entry := journal.Entries[index]
		if seen[entry.Path] {
			continue
		}
		seen[entry.Path] = true
		path := filepath.Join(root, filepath.FromSlash(entry.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if gitTracks(root, entry.Path) {
			plan.Blocked = append(plan.Blocked, entry.Path+i18n.T("gen_already_tracked"))
			continue
		}
		plan.Files = append(plan.Files, entry.Path)
	}
	sort.Strings(plan.Files)
	sort.Strings(plan.Blocked)
	return plan, nil
}

func gitTracks(root, path string) bool {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", filepath.FromSlash(path))
	cmd.Dir = root
	return cmd.Run() == nil
}

func removeRollbackFiles(root string, plan DevelopmentRollbackPlan) error {
	absRoot, _ := filepath.Abs(root)
	for _, relative := range plan.Files {
		path := filepath.Join(absRoot, filepath.FromSlash(relative))
		resolved, err := filepath.Abs(path)
		if err != nil || (resolved != absRoot && !strings.HasPrefix(resolved, absRoot+string(filepath.Separator))) {
			return i18n.Errf("gen_unsafe_path", relative)
		}
		if err := os.Remove(resolved); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	journal, err := loadGenerationJournal(root)
	if err != nil {
		return err
	}
	removed := map[string]bool{}
	for _, path := range plan.Files {
		removed[path] = true
	}
	kept := journal.Entries[:0]
	for _, entry := range journal.Entries {
		if !removed[entry.Path] {
			kept = append(kept, entry)
		}
	}
	journal.Entries = kept
	return writeGenerationJournal(root, journal)
}

var declarationNameNoise = regexp.MustCompile(`[^A-Za-z0-9]+`)
var legacyMigrationDeclaration = regexp.MustCompile(`\bfunc\s+Migration\s*\(\s*\)`)
var legacySeederDeclaration = regexp.MustCompile(`\bfunc\s+Seeder\s*\(\s*\)`)

func dynamicDeclarationName(prefix, filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	parts := strings.Fields(declarationNameNoise.ReplaceAllString(base, " "))
	var name strings.Builder
	name.WriteString(prefix)
	for _, part := range parts {
		if part == "" {
			continue
		}
		name.WriteByte('_')
		name.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return name.String()
}

func normalizeLegacyDeclarations(root string, state config.ConfigState) (int, error) {
	targets := []struct {
		folder  string
		prefix  string
		pattern *regexp.Regexp
	}{
		{state.Config.Output.Migrate, "Migration", legacyMigrationDeclaration},
		{state.Config.Output.Seed, "Seeder", legacySeederDeclaration},
	}
	changed := 0
	for _, target := range targets {
		folder := target.folder
		if !filepath.IsAbs(folder) {
			folder = filepath.Join(root, filepath.FromSlash(folder))
		}
		err := filepath.WalkDir(folder, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !target.pattern.Match(data) {
				return nil
			}
			name := dynamicDeclarationName(target.prefix, entry.Name())
			updated := target.pattern.ReplaceAll(data, []byte("func "+name+"()"))
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				return err
			}
			changed++
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return changed, err
		}
	}
	return changed, nil
}
