// Command docscheck keeps a repository's documentation and its code honest
// with each other. It fails on the drift that mechanical checks can catch, so
// that review time goes to the drift they cannot:
//
//   - a Markdown link whose file or heading does not exist;
//   - a `See docs/<file>.md#<heading>` citation in a Go comment that does not resolve;
//   - a citation of docs/research/, which is a dated snapshot;
//   - a path in backticks in AGENTS.md or README.md that does not exist and
//     is not marked "(planned)";
//   - a `make <target>` in a document that the Makefile lacks (skills vendored
//     through .agents/skills/shared.sum are exempt: their targets are examples);
//   - a required-Go-version claim in a root file that disagrees with go.mod;
//   - a work-item identifier (story, epic, ticket) in a document or in code
//     (skills are exempt: their examples may show one on purpose);
//   - a file in docs/research/ whose name does not start with a date;
//   - an AGENTS.md longer than the limit.
//
// It also lists headings in docs/ that no code cites, as information, because
// a design section nothing points at is either wrong or unnecessary.
//
// Copy this file into scripts/docscheck/ in a repository and run it from
// `make docs-check`. It uses only the standard library and takes the
// repository root as its working directory. Flags adjust the few things that
// differ between repositories; the conventions themselves do not change.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	// linkRe finds a Markdown link. The target is checked only when it is
	// relative; URLs and mailto are someone else's problem.
	linkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	// citationRe finds a pointer from Go code into docs/. The heading is
	// optional: a bare file citation only has to name a file that exists.
	citationRe = regexp.MustCompile(`//\s*See\s+(docs/[^\s#,.]+(?:\.[a-z]+)?)(?:#([A-Za-z0-9_-]+))?`)
	// pathRe finds a backticked token that looks like a repository path: it
	// contains a slash or starts with a dot, and is not a URL, flag or variable.
	pathRe = regexp.MustCompile("`([^`\\s]+)`")
	// makeRe matches a command, not prose: `make target` in backticks, or a
	// line in a fenced block that starts with make.
	makeRe     = regexp.MustCompile("`make ([A-Za-z0-9_.-]+)")
	makeLineRe = regexp.MustCompile(`^\s*make ([A-Za-z0-9_.-]+)`)
	// goVerRe matches a claim about the required Go version, not history:
	// "requires Go 1.26", "Go 1.26 or later", "Go 1.26+".
	goVerRe  = regexp.MustCompile(`(?i)(?:requires?|needs?|minimum|at least)\s+Go 1\.(\d+)|\bGo 1\.(\d+)(?:\.\d+)?(?:\+| or (?:later|newer))`)
	targetRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+):`)
	modGoRe  = regexp.MustCompile(`(?m)^go 1\.(\d+)`)
	headRe   = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	slugDrop = regexp.MustCompile(`[^\p{L}\p{N} -]`)
	dateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)
)

var (
	workItem   = flag.String("workitem", `\b[SE]-\d{3,4}\b`, "regexp for a work-item id that must not appear in docs or code; empty disables")
	agentsMax  = flag.Int("agents-max", 200, "maximum lines in AGENTS.md")
	skipPaths  = flag.String("skip", ".plan,.cache,node_modules,vendor,bin,dist", "comma-separated top-level dirs to ignore")
	listUncite = flag.Bool("uncited", true, "list docs/ headings nothing in Go cites (information only)")
)

type problem struct {
	file string
	line int
	msg  string
}

type checker struct {
	problems []problem
	headings map[string]map[string]int // file -> slug -> line
	cited    map[string]bool           // "file#slug"
	targets  map[string]bool
	skip     map[string]bool
	shared   []string // paths vendored from the skills repository
}

func main() {
	flag.Parse()
	c := &checker{headings: map[string]map[string]int{}, cited: map[string]bool{}, targets: map[string]bool{}, skip: map[string]bool{}}
	for _, s := range strings.Split(*skipPaths, ",") {
		if s != "" {
			c.skip[s] = true
		}
	}
	c.loadMakefile()
	c.loadShared()
	docs := c.markdownFiles()
	goFiles := c.goFiles()
	for _, d := range docs {
		c.checkMarkdown(d)
	}
	for _, g := range goFiles {
		c.checkGo(g)
	}
	c.checkResearchNames()
	c.checkAgentsLength()
	sort.Slice(c.problems, func(i, j int) bool {
		if c.problems[i].file != c.problems[j].file {
			return c.problems[i].file < c.problems[j].file
		}
		return c.problems[i].line < c.problems[j].line
	})
	for _, p := range c.problems {
		fmt.Printf("%s:%d: %s\n", p.file, p.line, p.msg)
	}
	if *listUncite {
		c.reportUncited()
	}
	if len(c.problems) > 0 {
		fmt.Printf("docscheck: %d problem(s)\n", len(c.problems))
		os.Exit(1)
	}
	fmt.Println("docscheck: ok")
}

func (c *checker) fail(file string, line int, format string, args ...any) {
	c.problems = append(c.problems, problem{file, line, fmt.Sprintf(format, args...)})
}

// walk visits every file under the root, skipping hidden and ignored dirs
// except .agents, where skills live.
func (c *checker) walk(fn func(path string)) {
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != "." && (c.skip[path] || (strings.HasPrefix(name, ".") && name != ".agents")) {
				return filepath.SkipDir
			}
			return nil
		}
		fn(path)
		return nil
	})
}

func (c *checker) markdownFiles() []string {
	var out []string
	c.walk(func(p string) {
		if strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
	})
	return out
}

func (c *checker) goFiles() []string {
	var out []string
	c.walk(func(p string) {
		if strings.HasSuffix(p, ".go") {
			out = append(out, p)
		}
	})
	return out
}

func (c *checker) loadMakefile() {
	f, err := os.Open("Makefile")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := targetRe.FindStringSubmatch(sc.Text()); m != nil {
			c.targets[m[1]] = true
		}
	}
}

// loadShared reads the paths vendored from the skills repository. A shared
// skill is generic by construction, so a make target it names is an example,
// not a claim about this Makefile; those checks skip it.
func (c *checker) loadShared() {
	f, err := os.Open(".agents/skills/shared.sum")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 3 && !strings.HasPrefix(fields[0], "#") {
			c.shared = append(c.shared, fields[0]+"/")
		}
	}
}

func (c *checker) isShared(path string) bool {
	for _, p := range c.shared {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// slugify applies GitHub's anchor rule: lowercase, drop everything but
// letters, digits, spaces and hyphens, then spaces become hyphens.
func slugify(text string) string {
	s := slugDrop.ReplaceAllString(strings.ToLower(text), "")
	return strings.ReplaceAll(s, " ", "-")
}

// headingsOf parses a Markdown file's headings once, skipping fenced code,
// with duplicate slugs suffixed -1, -2 the way GitHub does.
func (c *checker) headingsOf(path string) map[string]int {
	if h, ok := c.headings[path]; ok {
		return h
	}
	out := map[string]int{}
	f, err := os.Open(path)
	if err != nil {
		c.headings[path] = out
		return out
	}
	defer f.Close()
	seen := map[string]int{}
	inFence, n := false, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		n++
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := headRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		slug := slugify(m[1])
		if k := seen[slug]; k > 0 {
			seen[slug]++
			slug = fmt.Sprintf("%s-%d", slug, k)
		} else {
			seen[slug] = 1
		}
		out[slug] = n
	}
	c.headings[path] = out
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func (c *checker) checkMarkdown(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	isRoot := !strings.Contains(path, "/")
	inResearch := strings.HasPrefix(path, "docs/research/")
	var wi *regexp.Regexp
	if *workItem != "" && !inResearch {
		wi = regexp.MustCompile(*workItem)
	}
	modGo := ""
	if b, err := os.ReadFile("go.mod"); err == nil {
		if m := modGoRe.FindSubmatch(b); m != nil {
			modGo = string(m[1])
		}
	}
	// Work-item ids are a rule for documents and code, not for skills, whose
	// examples may show one on purpose.
	checkWI := wi != nil && !strings.HasPrefix(path, ".agents/")
	inFence := false
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if checkWI {
			if m := wi.FindString(line); m != "" {
				c.fail(path, n, "work-item id %q; describe the work, don't cite the tracker", m)
			}
		}
		if len(c.targets) > 0 && !c.isShared(path) {
			var found [][]string
			if inFence {
				found = makeLineRe.FindAllStringSubmatch(line, -1)
			} else {
				found = makeRe.FindAllStringSubmatch(line, -1)
			}
			for _, m := range found {
				if !c.targets[m[1]] {
					c.fail(path, n, "make target %q is not in the Makefile", m[1])
				}
			}
		}
		if modGo != "" && isRoot {
			for _, m := range goVerRe.FindAllStringSubmatch(line, -1) {
				v := m[1] + m[2]
				if v != modGo {
					c.fail(path, n, "says Go 1.%s but go.mod says 1.%s", v, modGo)
				}
			}
		}
		if inFence {
			continue
		}
		for _, m := range linkRe.FindAllStringSubmatch(line, -1) {
			c.checkLink(path, n, m[1])
		}
		if isRoot && !strings.Contains(line, "(planned)") {
			for _, m := range pathRe.FindAllStringSubmatch(line, -1) {
				c.checkPath(path, n, m[1])
			}
		}
	}
}

func (c *checker) checkLink(path string, n int, target string) {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return
	}
	file, anchor, _ := strings.Cut(target, "#")
	var abs string
	switch {
	case file == "":
		abs = path
	case strings.HasPrefix(file, "/"):
		abs = strings.TrimPrefix(file, "/")
	default:
		abs = filepath.Join(filepath.Dir(path), file)
	}
	if !exists(abs) {
		c.fail(path, n, "link target %q does not exist", target)
		return
	}
	if anchor == "" || !strings.HasSuffix(abs, ".md") {
		return
	}
	if _, ok := c.headingsOf(abs)[strings.ToLower(anchor)]; !ok {
		c.fail(path, n, "link anchor %q is not a heading in %s", anchor, abs)
	}
}

// checkPath treats a backticked token as a repository path when it ends in a
// slash (a directory claim), when its first segment is a top-level entry that
// exists, or when it is a dotfile with a slash or a second dot. Import paths,
// domains, URLs, flags, variables, globs and placeholders are left alone.
func (c *checker) checkPath(path string, n int, tok string) {
	for _, bad := range []string{"://", "<", ">", "*", "$", "~", "{", "@", " "} {
		if strings.Contains(tok, bad) {
			return
		}
	}
	if strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "/") {
		return
	}
	file, _, _ := strings.Cut(tok, "#")
	file = strings.TrimSuffix(strings.TrimSuffix(file, ":"), ".")
	first, _, hasSlash := strings.Cut(file, "/")
	candidate := strings.HasSuffix(file, "/") ||
		(hasSlash && exists(first) && !strings.Contains(first, ".")) ||
		(strings.HasPrefix(file, ".") && (hasSlash || strings.Count(file, ".") > 1))
	if !candidate || exists(file) {
		return
	}
	c.fail(path, n, "path `%s` does not exist (say \"(planned)\" on the line if it is intended)", tok)
}

func (c *checker) checkGo(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var wi *regexp.Regexp
	if *workItem != "" {
		wi = regexp.MustCompile(*workItem)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		if wi != nil {
			if m := wi.FindString(line); m != "" {
				c.fail(path, n, "work-item id %q in code", m)
			}
		}
		m := citationRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		file, anchor := m[1], m[2]
		if !strings.HasSuffix(file, ".md") {
			file += ".md"
		}
		if !exists(file) {
			c.fail(path, n, "cites %s, which does not exist", file)
			continue
		}
		if strings.HasPrefix(file, "docs/research/") {
			c.fail(path, n, "cites %s; research is a dated snapshot and is never cited", file)
			continue
		}
		if anchor == "" {
			continue
		}
		if _, ok := c.headingsOf(file)[strings.ToLower(anchor)]; !ok {
			c.fail(path, n, "cites %s#%s, which is not a heading there", file, anchor)
			continue
		}
		c.cited[file+"#"+strings.ToLower(anchor)] = true
	}
}

func (c *checker) checkResearchNames() {
	entries, err := os.ReadDir("docs/research")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "README.md" {
			continue
		}
		if !dateRe.MatchString(e.Name()) {
			c.fail("docs/research/"+e.Name(), 1, "research file name must start with YYYY-MM-DD-")
		}
	}
}

func (c *checker) checkAgentsLength() {
	data, err := os.ReadFile("AGENTS.md")
	if err != nil {
		return
	}
	lines := strings.Count(string(data), "\n")
	if lines > *agentsMax {
		c.fail("AGENTS.md", lines, "%d lines; the limit is %d. Move procedure to a skill, design to docs/", lines, *agentsMax)
	}
}

// reportUncited lists docs/ headings no Go file cites. Only design documents
// count: the index, research and reference material are read, not cited.
func (c *checker) reportUncited() {
	if len(c.cited) == 0 {
		return
	}
	var lines []string
	for _, d := range c.markdownFiles() {
		if !strings.HasPrefix(d, "docs/") || strings.HasPrefix(d, "docs/research/") || strings.HasPrefix(d, "docs/reference/") || filepath.Base(d) == "README.md" {
			continue
		}
		for slug, ln := range c.headingsOf(d) {
			if !c.cited[d+"#"+slug] {
				lines = append(lines, fmt.Sprintf("%s:%d: #%s", d, ln, slug))
			}
		}
	}
	if len(lines) == 0 {
		return
	}
	sort.Strings(lines)
	fmt.Printf("uncited headings (information, not failures):\n")
	for _, l := range lines {
		fmt.Println("  " + l)
	}
}
