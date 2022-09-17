package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/google/go-github/v47/github"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

var rootCmd = &cobra.Command{
	Use:   "ghs",
	Short: "GitHub Search wraps the API to be closer to grep/ag/etc semantics",
	Run:   execute,
}

var cfgFile = ""
var org = ""
var lang = ""
var verbose = false

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&cfgFile, "config", cfgFile, "config file (default is $HOME/.ghs.yaml or $PWD/.ghs.yaml)")
	rootCmd.Flags().StringVarP(&org, "org", "o", org, "organization to scope searches to")
	rootCmd.Flags().StringVarP(&lang, "lang", "l", lang, "shortcut for scoping search with language:<lang>")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "display debugging messages on stderr")

	// Pathname vs. repo vs. URL vs. filename only?
	// Repo?
	// only repo?

	// Maybe have an interactive option that's just a glorified less with the
	// ability to toggle fully-qualified repo + path + whatever metadata without
	// re-issuing the query.

	// Tests against the highlighting, indicing, etc

	// Put everything into less GraphQL queries

	// Common filters like...
	// - user: stars: extension: repo: size: path: filename:?

	// Login flow showing a stopwatch followed with "Wow, you're fast!"

	viper.BindPFlags(rootCmd.Flags())
	viper.SetEnvPrefix("ghs")
	viper.BindEnv("TOKEN")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		return
	}

	viper.SetConfigName(".ghs")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		fatalf("create a valid config at ~/.ghs or ./.ghs: %v", err)
	}
}

// barebones logging
func v(format string, a ...any) {
	if !viper.GetBool("verbose") {
		return
	}
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprintln(os.Stderr)
}

func w(format string, a ...any) {
	fmt.Fprintf(os.Stderr, color.YellowString(format, a...))
	fmt.Fprintln(os.Stderr)
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
	os.Exit(1)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getClient(ctx context.Context) *github.Client {
	token := viper.GetString("TOKEN")
	if token == "" {
		fatalf("please set TOKEN in ~/.ghs.yaml or GHS_TOKEN in env")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return github.NewClient(tc)
}

func getGQL(ctx context.Context) *githubv4.Client {
	token := viper.GetString("TOKEN")
	if token == "" {
		fatalf("please set TOKEN in ~/.ghs.yaml or GHS_TOKEN in env")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return githubv4.NewClient(tc)

}

func makeQuery(args []string) string {
	var query string

	org := viper.GetString("org")
	lang := viper.GetString("lang")
	if org != "" {
		query += "org:" + org + " "
	}
	if lang != "" {
		query += "language:" + lang + " "
	}
	return strings.Join(args, " ") + " " + query
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

func fetchText(ctx context.Context, owner, name, branch, path string) (string, error) {
	client := getGQL(ctx)
	vars := map[string]interface{}{
		"owner": githubv4.String(owner),
		"name":  githubv4.String(name),
		"oid":   githubv4.String(fmt.Sprintf("%s:%s", branch, path)),
	}
	var q struct {
		Repository struct {
			Object struct {
				Blob struct {
					Text string
				} `graphql:"... on Blob"`
			} `graphql:"object(expression: $oid)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}
	err := client.Query(ctx, &q, vars)
	if err != nil {
		return "", err
	}
	return q.Repository.Object.Blob.Text, nil
}

// fetchAllContents from search results
//
// The context GitHub chooses to give with a search query is odd. It's
// not based on line boundaries. It may be a partial first line, full
// second line, and a partial third line — where the actual match is.
//
// To match grep-like expectations, fetch all contents so we can fill-
// in the gaps.
//
// key: owner:name:branch:path
// value: file content
func fetchAllContents(ctx context.Context, searchResults []*github.CodeResult) map[string]string {

	// Prepare graphql vars
	contentArgs := [][4]string{}
	for _, r := range searchResults {
		repo := r.GetRepository()
		contentArgs = append(contentArgs, [4]string{
			repo.GetOwner().GetLogin(),
			repo.GetName(),
			// TODO: DefaultBranch exists on repos but isn't included in the search results.
			// Not sure if we should make more I/O trips for it, esp. since GH loves ratelimititng.
			// Can make configurable globally / overridable per-repo/org at the least.
			"master",
			r.GetPath(),
		})
	}

	// Sync writes to map
	content := map[string]string{}
	contentCh := make(chan struct {
		key string
		val string
	})

	go func() {
		for s := range contentCh {
			content[s.key] = s.val
		}
	}()

	wg := sync.WaitGroup{}
	start := time.Now()

	// Replies range between 160-350ms, with p99 up to 2s.
	//
	// Because we're using a strongly-typed graphql lib, we must define the
	// entire query up front. If doing one query per network rt becomes an
	// issue, we can pre-define 25-100 named queries in a struct and disassemble
	// after.
	for _, args := range contentArgs {
		wg.Add(1)
		go func(args [4]string) {
			defer wg.Done()

			start := time.Now()
			defer func() {
				v("%s\t%v", time.Since(start), args)
			}()

			text, err := fetchText(ctx, args[0], args[1], args[2], args[3])
			if err != nil {
				// TODO: Unsure if we should degrade or not. Deciding not to for now to ship.
				// Should also bubble up instead of mid-program panic but... later.
				fatalf(fmt.Sprint(err))
			}
			contentCh <- struct {
				key string
				val string
			}{
				fmt.Sprintf("%s:%s:%s:%s", args[0], args[1], args[2], args[3]), text,
			}
		}(args)
	}

	wg.Wait()
	v("Fetching contents took %s in aggregate", time.Since(start))
	v("Fetched contents for %d files", len(content))
	return content
}

func performSearch(ctx context.Context, query string) (*github.CodeSearchResult, error) {
	start := time.Now()
	defer func() {
		v("Performing search took %s", time.Since(start))
	}()

	// TODO: archived?
	// TODO: Set number of matches. Need to support paging.
	client := getClient(ctx)
	opts := &github.SearchOptions{TextMatch: true}
	res, _, err := client.Search.Code(ctx, query, opts)
	return res, err
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

func createMatches(res []*github.CodeResult, fileContents map[string]string) []match {
	matches := []match{}
	for _, r := range res {
		owner := r.GetRepository().GetOwner().GetLogin()
		repo := r.GetRepository().GetName()
		branch := "master"
		path := r.GetPath()

		fullText := fileContents[fmt.Sprintf("%s:%s:%s:%s", owner, repo, branch, path)]

		for _, tm := range r.TextMatches {
			frag := tm.GetFragment()

			// Locate fragment in full text
			fragIdx := strings.Index(fullText, frag)
			if fragIdx == -1 {
				w("Couldn't find fragment in full text: %s, %s\nFragment: %s", repo, path, frag)
				continue
			}

			// Colorize matches in the full text
			matchIndices := []int{}
			for _, match := range tm.Matches {
				i, j := match.Indices[0], match.Indices[1]
				// Translate match indices in frag to in full-text
				//
				// This makes it much easier to show only match line vs. contextual lines
				i += fragIdx
				j += fragIdx

				matchIndices = append(matchIndices, i)
				fullText = fullText[:i] + color.RedString(fullText[i:j]) + fullText[j:]
			}

			// Locate the entire line of the matches
			for _, idx := range matchIndices {
				start := idx
				for {
					start--
					if fullText[start] == '\n' || start == 0 {
						break
					}
				}

				// start points to '\n', so move forward by 1
				start += 1
				end := strings.IndexByte(fullText[start:], '\n')
				if end == -1 {
					end = len(fullText)
				} else {
					// end is currently relative to the [start:] slice — fix
					end += start
				}

				matches = append(matches, match{
					owner:  owner,
					repo:   repo,
					branch: branch,
					path:   path,
					lineno: strings.Count(fullText[:start], "\n"),
					colno:  idx - start,
					text:   fullText[start:end],
				})
			}
		}
	}
	return matches
}

func execute(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	query := makeQuery(args)
	v("Query: %s", query)

	res, err := performSearch(ctx, query)
	if err != nil {
		fatalf(fmt.Sprint(err))
	}

	fileContents := fetchAllContents(ctx, res.CodeResults)
	matches := createMatches(res.CodeResults, fileContents)
	for _, m := range matches {
		fmt.Print(color.BlueString(m.path))
		fmt.Print(":")
		fmt.Print(color.GreenString(fmt.Sprint(m.lineno)))
		fmt.Print(":")
		fmt.Print(color.GreenString(fmt.Sprint(m.colno)))
		fmt.Print(": ")
		fmt.Println(m.text)
	}
}
