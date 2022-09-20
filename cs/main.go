// This is the high-level flow:
//
// - Run search with given terms
// - Coerce results into our own minimal structure
// - Look up default branch names for every returned repo
//   - This costs ~300-500ms so you can skip this step with 'set-default-branch'
// - Fetch file contents for every returned Path
// - Overlay colorized text matches onto the contents
// - Write each line to stdout
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/go-github/v47/github"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"mvdan.cc/gofumpt/format"
)

var rootCmd = &cobra.Command{
	Use:   "cs [terms] [flags]",
	Short: "Codesearch wraps the GitHub API to be closer to grep/ag/etc semantics",
	Long: `
Codesearch wraps the GitHub API to be closer to grep/ag/et al semantics

Positional args are merged into a single string and used as the search term. Refer to
GitHub's documentation for nuances: https://docs.github.com/en/search-github/searching-on-github/searching-code

While we've done our best, GitHub can be harsh with ratelimiting. If your org
has consistent branch names, consider running 'cs set-default-branch' to
alleviate some pressure.
	`,
	Run:  execute,
	Args: cobra.MinimumNArgs(1),
}

// Defaults should be filled in by pflags vs. zero-values
var flags = struct {
	org      string
	lang     string
	filename string
	path     string
	ext      string

	limit         int
	after         int
	before        int
	context       int
	count         bool
	printURLs     bool
	onlyFiles     bool
	onlyRepos     bool
	onlyFullNames bool

	contentOnly bool
	formatStr   string

	cfgFile   string
	verbose   bool
	showQuery bool
	dumpData  bool
	tabWidth  int

	// includeArchived bool
}{}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.Flags().IntVar(&flags.limit, "limit", 30, "limit the number of matches queried and displayed")

	rootCmd.Flags().StringVarP(&flags.org, "org", "o", "", "scope search with a single organization:[org]")
	rootCmd.Flags().StringVar(&flags.lang, "lang", "", "scope search with a single language:[lang]")
	rootCmd.Flags().StringVarP(&flags.filename, "filename", "f", "", "scope search by filename")
	rootCmd.Flags().StringVarP(&flags.path, "path", "p", "", "scope search by the path files are in")
	rootCmd.Flags().StringVarP(&flags.ext, "ext", "x", "", "scope search by file extension")

	rootCmd.Flags().IntVarP(&flags.after, "after-context", "A", 0, "print [num] lines of trailing context after each match")
	rootCmd.Flags().IntVarP(&flags.before, "before-context", "B", 0, "print [num] lines of leading context before each match")
	rootCmd.Flags().IntVarP(&flags.context, "context", "C", 0, "print [num] lines of context before and after each match")
	rootCmd.Flags().BoolVarP(&flags.count, "count", "c", false, "print only a count of matches")

	rootCmd.Flags().BoolVarP(&flags.printURLs, "url", "u", false, "print URLs to the selected line as the prefix before text")
	rootCmd.Flags().BoolVarP(&flags.onlyFiles, "files-only", "l", false, "print only filenames of matches to stdout")
	rootCmd.Flags().BoolVar(&flags.onlyRepos, "repos-only", false, "print only repository names containing matches to stdout")
	rootCmd.Flags().BoolVar(&flags.onlyFullNames, "full-names-only", false, "print only fully-qualified repo names to stdout (your/repo path/to/README.md)")

	rootCmd.Flags().StringVar(&flags.cfgFile, "config", "", "overrides location of the config file")
	rootCmd.Flags().BoolVarP(&flags.verbose, "verbose", "v", false, "prints verbose messages to stderr for debugging")
	rootCmd.Flags().BoolVarP(&flags.showQuery, "show-query", "q", false, "show the search terms we would send to GitHub and exit")
	rootCmd.Flags().BoolVar(&flags.dumpData, "dump", false, "dump result structures to stdout")

	defaultFmt := "$repo_path:$lineno: $text"
	rootCmd.Flags().StringVar(&flags.formatStr, "format", defaultFmt, "custom format string (variables: $owner, $repo_name, $repo_path, $path, $lineno, $colno, $text, $url)")
	rootCmd.Flags().BoolVar(&flags.contentOnly, "content", false, "print only the text results, nothing else")

	rootCmd.Flags().IntVar(&flags.tabWidth, "tabwidth", 2, "number of spaces to display tabs as")

	// TODO: Unfortunately only cs.github.com has archive term support at the moment
	// rootCmd.Flags().BoolVarP(&flags.includeArchived, "archived", "a", false, "include results from archived repositories")

	viper.BindPFlag("org", rootCmd.Flags().Lookup("org"))
	viper.BindPFlag("format", rootCmd.Flags().Lookup("format"))
	viper.BindPFlag("tabwidth", rootCmd.Flags().Lookup("tabwidth"))
	viper.BindPFlag("url", rootCmd.Flags().Lookup("url"))

	// TODO: have an interactive option that's just a glorified `less` with the
	// ability to toggle fully-qualified repo + path + whatever metadata without
	// re-issuing the query.
	//
	// Even --context could be done since we do that in-memory vs. in-search.

	// TODO: Common filters like these?
	// - user: stars: extension: repo: size: path: filename:?
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	query := makeQuery(args)
	if flags.showQuery {
		fmt.Println(query)
		return
	}
	v("Query: %s", query)

	res, err := performSearch(ctx, query, flags.limit)
	if err != nil {
		fatalf(fmt.Sprint(err))
	}

	httpClient := getAuthenticatedHTTP(ctx)

	searchResult := coerceResults(res)

	// TODO: With pagination, it might make sense to do this as each result comes in
	if flags.onlyFiles {
		printFiles(searchResult)
		return
	}

	if flags.onlyFullNames {
		printFullNames(searchResult)
		return
	}

	if flags.onlyRepos {
		printRepos(searchResult)
		return
	}

	defaultBranches := getDefaultBranches(httpClient, searchResult)

	var fullText FullText
	if len(searchResult) >= 100 {
		fullText = paginateFullText(httpClient, searchResult, defaultBranches)
	} else {
		fullText = getFullText(httpClient, searchResult, defaultBranches)
	}

	if flags.dumpData {
		dumpData(searchResult, defaultBranches, fullText)
		return
	}

	matches := createMatches(searchResult, fullText, defaultBranches)
	for _, m := range matches {
		if flags.contentOnly {
			fmt.Println(m.text)
			continue
		}

		if flags.printURLs {
			fmt.Print(color.BlueString(m.url()))
			fmt.Print(":")
			fmt.Println(m.text)
			continue
		}

		out := flags.formatStr

		// TODO: Probably not ideal having default colors but unsure how to do
		// it without it being overwieldly atm.
		out = strings.ReplaceAll(out, "$owner", m.owner)
		out = strings.ReplaceAll(out, "$repo_name", m.repo)
		out = strings.ReplaceAll(out, "$repo_path", fmt.Sprint(
			color.New(color.FgBlue).Sprint(m.repo),
			color.BlueString("/"+m.path),
		))
		out = strings.ReplaceAll(out, "$path", color.BlueString(m.path))
		out = strings.ReplaceAll(out, "$lineno", color.GreenString(fmt.Sprint(m.lineno)))
		out = strings.ReplaceAll(out, "$colno", color.GreenString(fmt.Sprint(m.colno)))
		out = strings.ReplaceAll(out, "$text", m.text)
		out = strings.ReplaceAll(out, "$url", color.BlueString(m.url()))
		fmt.Println(out)
	}
}

type FileKey struct {
	Owner string
	Name  string
	Path  string
}

func (f *FileKey) String() string {
	return fmt.Sprintf("%s/%s %s", f.Owner, f.Name, f.Path)
}

func (f *FileKey) RepoString() string {
	return fmt.Sprintf("%s/%s", f.Owner, f.Name)
}

type TextMatch struct {
	Fragment string
	Indices  [][2]int
}

type SearchResult map[FileKey][]TextMatch

// coerceResults from GitHub -> our own structures
//
// The return from the API is a bit messy with pointers everywhere, retrieval
// methods, etc. Coercing the exact info we want as early as possible makes
// testing + reasoning easier.
func coerceResults(fromGitHub []*github.CodeResult) SearchResult {
	coerced := SearchResult{}
	for _, cr := range fromGitHub {
		repo := cr.GetRepository()
		key := FileKey{
			Owner: repo.GetOwner().GetLogin(),
			Name:  repo.GetName(),
			Path:  cr.GetPath(),
		}

		fragments := []TextMatch{}
		for _, tm := range cr.TextMatches {
			// GitHub returns text matches for search terms like
			// filename:blah.txt -> "blah.txt". We only care about the source
			// for highlights.
			if tm.GetObjectType() != "FileContent" || tm.GetProperty() != "content" {
				continue
			}
			indices := [][2]int{}
			for _, match := range tm.Matches {
				indices = append(indices, [2]int{match.Indices[0], match.Indices[1]})
			}
			fragments = append(fragments, TextMatch{
				Fragment: tm.GetFragment(),
				Indices:  indices,
			})
		}

		coerced[key] = fragments
	}
	return coerced
}

func makeQuery(args []string) string {
	var query string

	org := viper.GetString("org")
	if org != "" {
		query += "org:" + org + " "
	}

	if lang := flags.lang; lang != "" {
		query += "language:" + lang + " "
	}

	if filename := flags.filename; filename != "" {
		query += "filename:" + filename + " "
	}

	if path := flags.path; path != "" {
		query += "path:" + path + " "
	}

	if ext := flags.ext; ext != "" {
		query += "extension:" + ext + " "
	}

	return strings.Join(args, " ") + " " + query
}

func performSearch(ctx context.Context, query string, limit int) ([]*github.CodeResult, error) {
	start := time.Now()
	defer func() {
		v("Performing search took %s", time.Since(start))
	}()

	client := github.NewClient(getAuthenticatedHTTP(ctx))
	v("User-Agent: %s", client.UserAgent)

	opts := &github.SearchOptions{TextMatch: true}
	if limit > 0 {
		opts.PerPage = limit
	}

	// GitHub only allows 100 results. We'll make things right with paging.
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}

	opts.Page = 1
	remaining := limit
	results := []*github.CodeResult{}
	for remaining > 0 {
		v("Page: %d", opts.Page)
		res, _, err := client.Search.Code(ctx, query, opts)
		if err != nil {
			return nil, err
		}

		if flags.count {
			fmt.Println(res.GetTotal())
			os.Exit(0)
		}

		results = append(results, res.CodeResults...)
		rx := len(res.CodeResults)
		remaining -= rx
		opts.Page += 1

		soFar := limit - remaining
		v("Fetched: %d/%d", soFar, res.GetTotal())
		total := res.GetTotal()
		if total <= soFar {
			break
		}
	}

	return results, nil
}

func indexAllByte(s string, c byte) []int {
	indices := []int{}
	for i := range s {
		if s[i] == c {
			indices = append(indices, i)
		}
	}
	return indices
}

// match corresponds to one line written to the terminal
type match struct {
	owner  string
	repo   string
	branch string
	path   string

	lineno int
	colno  int
	text   string
}

func (m *match) repoString() string {
	return fmt.Sprintf("%s/%s", m.owner, m.repo)
}

func (m *match) url() string {
	return fmt.Sprintf(
		"https://github.com/%s/blob/%s/%s#L%d",
		m.repoString(),
		m.branch,
		m.path,
		m.lineno,
	)
}

type FileKeys []FileKey

func (f FileKeys) Len() int {
	return len(f)
}

func (f FileKeys) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

func (f FileKeys) Less(i, j int) bool {
	fi := f[i]
	iName := fmt.Sprintf("%s-%s-%s", fi.Owner, fi.Name, fi.Path)
	fj := f[j]
	jName := fmt.Sprintf("%s-%s-%s", fj.Owner, fj.Name, fj.Path)
	return iName < jName
}

func createMatches(searchResult SearchResult, fullText FullText, defaultBranches map[string]string) []match {
	// Consistent sort.
	// It's also easier to read when things gradually follow similar lines optically.
	sortedKeys := []FileKey{}
	for key := range searchResult {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Sort(FileKeys(sortedKeys))

	matches := []match{}
	var shown int
	for _, key := range sortedKeys {
		textMatches := searchResult[key]
		content := fullText.Values[key]

		for _, tm := range textMatches {

			if flags.limit > 0 && shown >= flags.limit {
				break
			}

			// Locate fragment in full text
			fragIdx := strings.Index(content, tm.Fragment)
			if fragIdx == -1 && fullText.Truncated[key] {
				w("file content truncated in GitHub reply: %s/%s %s", key.Owner, key.Name, key.Path)
				continue
			} else if fragIdx == -1 {
				w("couldn't find search term in file content: %s/%s %s", key.Owner, key.Name, key.Path)
				v("search term:%v", tm.Fragment)
				continue
			}

			// Bump how many unique fragments we've seen.
			// Doing this before we do line-math because we don't want limit to
			// affect contextual flags. (-A, -B, -C)
			shown++

			// Used as a starting point to isolated match *lines* from match *tokens*
			startPositions := []int{}

			// Colorize matches in the full text
			var ansiOverhead int
			for _, indices := range tm.Indices {
				// Translate match indices in fragment to in full-text This
				// makes it much easier to show only match line OR
				// user-specified number of contextual lines
				i, j := indices[0], indices[1]
				i += fragIdx + ansiOverhead
				j += fragIdx + ansiOverhead
				startPositions = append(startPositions, i)

				highlight := color.RedString(content[i:j])
				content = content[:i] + highlight + content[j:]

				// Account for byte overhead as we're adding ANSI sequences
				//
				// When color is off, this should still work since the color
				// functions transparently stop applying color.
				//
				// Doing last, we only care about what the last iteration added in
				ansiOverhead += len(highlight) - len(content[i:j])
			}

			// These start positions may cohabitate the same line. We want to
			// avoid printing both out - we've already color highlighted
			// everything!
			foundLinenos := map[int]struct{}{}
			// Locate the entire line of the matches
			//
			// - Move backward from each starting point until finding a newline
			// - Increase by one to get first character of that line
			// - Repeat simultaneously in the forward direction
			for _, idx := range startPositions {
				var foundStart bool
				var foundEnd bool
				start := idx
				end := idx
				for {
					if content[start] == '\n' || start == 0 {
						foundStart = true
					}
					if !foundStart {
						start--
					}
					if content[end] == '\n' || end == len(content)-1 {
						foundEnd = true
					}
					if !foundEnd {
						end++
					}

					if foundStart && foundEnd {
						break
					}
				}

				start++
				end--

				lineno := strings.Count(content[:start], "\n") + 1
				if _, ok := foundLinenos[lineno]; ok {
					v("[%d] already processed this line, moving on!", lineno)
					continue
				}
				foundLinenos[lineno] = struct{}{}

				// Check if we need to show any extra lines contextual to the
				// matching one.
				//
				// Allow combining -C with flags -A and -B. The larger number just wins.
				var leading, trailing []string
				before := max(flags.before, flags.context)
				after := max(flags.after, flags.context)
				if before > 0 || after > 0 {
					leading, trailing = contextLines(content, lineno, before, after)
				}

				for i, l := range leading {
					matches = append(matches, match{
						owner:  key.Owner,
						repo:   key.Name,
						branch: defaultBranches[key.RepoString()],
						path:   key.Path,
						lineno: lineno - len(leading) + i,
						colno:  0,
						text:   shrinkTabs(l),
					})
				}

				matches = append(matches, match{
					owner:  key.Owner,
					repo:   key.Name,
					branch: defaultBranches[key.RepoString()],
					path:   key.Path,
					lineno: lineno,
					colno:  idx - start,
					text:   shrinkTabs(content[start : end+1]),
				})

				for i, l := range trailing {
					matches = append(matches, match{
						owner:  key.Owner,
						repo:   key.Name,
						branch: defaultBranches[key.RepoString()],
						path:   key.Path,
						lineno: lineno + i + 1,
						colno:  0,
						text:   shrinkTabs(l),
					})
				}
			}
		}
	}
	return matches
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// shrinkTabs into 2-width spaces
//
// The screen is cramped enough trying to fit repo context in without a monorepo
func shrinkTabs(s string) string {
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", flags.tabWidth))
}

func contextLines(content string, lineno, before, after int) (leading, trailing []string) {
	start := time.Now()
	defer func() {
		v("Finding contextual lines took %s", time.Since(start))
	}()

	lineIdx := lineno - 1
	lines := strings.Split(content, "\n")

	if beforeCnt := len(lines[:lineIdx]); before > beforeCnt {
		before = beforeCnt
	}
	if afterCnt := len(lines[lineIdx:]) - 1; after > afterCnt {
		after = afterCnt
	}

	if before > 0 {
		leading = lines[lineIdx-before : lineIdx]
	}
	if after > 0 {
		trailing = lines[lineIdx : lineIdx+after+1]
	}

	// Don't include the line itself
	if len(trailing) > 1 {
		trailing = trailing[1:]
	} else {
		trailing = []string{}
	}
	return leading, trailing
}

func printFiles(r SearchResult) {
	for key := range r {
		fmt.Println(key.Path)
	}
}

func printFullNames(r SearchResult) {
	for key := range r {
		fmt.Println(key.String())
	}
}

func printRepos(r SearchResult) {
	seen := map[string]struct{}{}
	for key := range r {
		s := key.RepoString()
		if _, ok := seen[s]; ok {
			continue
		}
		fmt.Println(s)
		seen[s] = struct{}{}
	}
}

// dumpData to stdout as functional code for easier test case making
func dumpData(searchResult SearchResult, defaultBranches map[string]string, fullText FullText) {
	gen := strings.ReplaceAll(fmt.Sprintf(`
package main

var searchResults = %#v

var defaultBranches = %#v

var fullText = %#v
		`, searchResult, defaultBranches, fullText), "main.", "")

	formatted, err := format.Source([]byte(gen), format.Options{ExtraRules: true})
	if err != nil {
		fatalf("failed trying to format dumped data: %v", err)
	}
	fmt.Println(string(formatted))
}
