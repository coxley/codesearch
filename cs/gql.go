// GraphQL specifics
//
// Unfortunately the strongly-typed gql libs for Go aren't perfect for
// dynamic-length queries. If the entire query structure isn't known at compile
// time, there's not a great way to model it.
//
// So we use text/template with a bit of care. The alternative is to make a
// bunch of queries in parallel. Let's err on the side of not increasing
// surface area for issues - especially since GitHub likes to ratelimit.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/viper"
)

func gqlURL() string {
	baseURL := viper.Get("base_url").(string)
	if strings.HasSuffix(baseURL, "/") {
		return baseURL + "graphql"
	}
	return baseURL + "/graphql"
}

type gqlRequest struct {
	Query     string `json:"query"`
	Variables string `json:"variables"`
}

var defaultBranchesTempl = `
query {
{{ range . }}
	{{printf "b%d" .Idx}}:repository(owner: "{{.Owner}}", name: "{{.Name}}") {
		owner {
			login
		}
		name
		defaultBranchRef {
			name
		}
	}
{{ end }}
}
`

// getDefaultBranches is a workaround of...
//   - Search API not including it in the response
//   - Search API supporting only searching the default branch
//   - The GraphQL API for file content requiring branch
//   - Needing the full file content because the Search API returns partial lines
func getDefaultBranches(client *http.Client, result SearchResult) map[string]string {
	// Create map with all equal values to avoid complexity in downstream functions
	if name := viper.GetString("defaultBranch"); name != "" {
		v("Using configured default branch for everything: %s", name)
		defaultBranches := map[string]string{}
		for key := range result {
			defaultBranches[fmt.Sprintf("%s/%s", key.Owner, key.Name)] = name
		}
		return defaultBranches
	}

	start := time.Now()
	defer func() {
		v("Getting default branches took %s", time.Since(start))
	}()

	// Generate template used for the GraphQL query
	type tmplData struct {
		Idx   int
		Owner string
		Name  string
	}
	data := []tmplData{}

	var i int
	seen := map[string]struct{}{}
	for key := range result {
		seenKey := fmt.Sprint(key.Owner, key.Name)
		if _, ok := seen[seenKey]; ok {
			continue
		}
		data = append(data, tmplData{i, key.Owner, key.Name})
		seen[seenKey] = struct{}{}
		i++
	}

	var query bytes.Buffer
	t := template.Must(template.New("branches").Parse(defaultBranchesTempl))
	err := t.Execute(&query, data)
	if err != nil {
		fatalf("failed to query branch data: %v", err)
	}

	gql, err := json.Marshal(gqlRequest{Query: query.String()})
	if err != nil {
		fatalf("failed to create gql request as json: %v", err)
	}

	resp, err := client.Post(gqlURL(), "application/json", bytes.NewReader(gql))
	if err != nil {
		fatalf("gql request to fetch branches failed: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fatalf("failed to read response body from gql: %v", err)
	}

	type gqlResponse struct {
		Data map[string]struct {
			Owner struct {
				Login string
			}
			Name             string
			DefaultBranchRef struct {
				Name string
			}
		}
	}

	var gr gqlResponse
	err = json.Unmarshal(b, &gr)
	if err != nil {
		fatalf("gql response failed to unmarshal: %v", err)
	}

	defaultBranches := map[string]string{}
	for _, repo := range gr.Data {
		fullName := fmt.Sprintf("%s/%s", repo.Owner.Login, repo.Name)
		defaultBranches[fullName] = repo.DefaultBranchRef.Name
	}
	return defaultBranches
}

var fullTextTempl = `
query {
	{{ range .}}
	{{printf "t%d" .Idx}}:repository(owner: "{{.Owner}}", name: "{{.Name}}") {
		object(expression:"{{.Branch}}:{{.Path}}") {
			... on Blob {
				text
				isTruncated
			}
		}
	}
{{ end }}
}
`

type FullText struct {
	Values map[FileKey]string
	// Github MAY truncate the contents of a file. Luckily it can tell us when
	// it happens.
	Truncated map[FileKey]bool
}

func paginateFullText(client *http.Client, result SearchResult, defaultBranches map[string]string) FullText {
	chunks := []SearchResult{}
	chunkSz := 100

	var i int
	cur := SearchResult{}
	for k, v := range result {
		if i == chunkSz {
			chunks = append(chunks, cur)
			cur = SearchResult{}
			i = 0
		}
		cur[k] = v
		i++
	}

	v("GQL pages to run: %d", len(chunks))

	res := FullText{Values: map[FileKey]string{}, Truncated: map[FileKey]bool{}}
	for page, chunk := range chunks {
		v("GQL Page: %d", page)

		pr := getFullText(client, chunk, defaultBranches)
		for k, v := range pr.Values {
			res.Values[k] = v
		}

		for k, v := range pr.Truncated {
			res.Truncated[k] = v
		}
	}
	return res
}

// getFullText is a workaround of...
//   - The Search API returning partial lines surrounding the matching terms
func getFullText(client *http.Client, result SearchResult, defaultBranches map[string]string) FullText {
	start := time.Now()
	defer func() {
		v("Getting full text of files took %s", time.Since(start))
	}()

	// Generate template used for the GraphQL query
	type tmplData struct {
		FileKey
		Branch string
		Idx    int
	}
	data := []tmplData{}

	// The 'Blob' model doesn't expose the path as a field... even though it is
	// used as the argument.
	//
	// Use this map to keep track of what FileKey is associated with which file.
	//
	// (We _could_ create a fully-qualified file name to use as the query alias, but
	//  it's fragile. GraphQL accepts a more limited set of characters for names than
	//  files can have. This is easier than having test cases around normalizing.)
	queryAliases := map[string]FileKey{}
	var i int
	for key := range result {
		data = append(data, tmplData{
			key,
			defaultBranches[fmt.Sprintf("%s/%s", key.Owner, key.Name)],
			i,
		})
		queryAliases[fmt.Sprintf("t%d", i)] = key
		i++
	}

	var query bytes.Buffer
	t := template.Must(template.New("fulltext").Parse(fullTextTempl))
	err := t.Execute(&query, data)
	if err != nil {
		fatalf("failed to query file data: %v", err)
	}

	qstr := query.String()
	v("full text gql: %s", qstr)

	gql, err := json.Marshal(gqlRequest{Query: qstr})
	if err != nil {
		fatalf("failed to create gql request as json: %v", err)
	}

	resp, err := client.Post(gqlURL(), "application/json", bytes.NewReader(gql))
	if err != nil {
		fatalf("gql request to fetch file contents failed: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fatalf("failed to read response body from gql: %v", err)
	}

	type gqlResponse struct {
		Data map[string]struct {
			Object struct {
				Text        string
				IsTruncated bool
			}
		}
	}

	var gr gqlResponse
	err = json.Unmarshal(b, &gr)
	if err != nil {
		fatalf("gql response failed to unmarshal: %v", err)
	}

	fullText := FullText{Values: map[FileKey]string{}, Truncated: map[FileKey]bool{}}
	for alias, repo := range gr.Data {
		key := queryAliases[alias]
		v("gql alias to filename: %s => %s", alias, key.String())
		fullText.Values[key] = repo.Object.Text
		if repo.Object.IsTruncated {
			fullText.Truncated[key] = true
		}
	}
	return fullText
}
