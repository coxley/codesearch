package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/google/go-github/v47/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

// barebones logging
func v(format string, a ...any) {
	if !flags.verbose {
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

func getAuthenticatedHTTP(ctx context.Context) *http.Client {
	if token == "" {
		fatalf("please run %s set-token", os.Args[0])
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	return oauth2.NewClient(ctx, ts)
}

func githubClient(ctx context.Context) (*github.Client, error) {
	baseURL := viper.GetString("base_url")
	return github.NewEnterpriseClient(baseURL, baseURL, getAuthenticatedHTTP(ctx))
}

// Create non-API links (repos, files)
func makeGithubSiteURL(path string) string {
	// Github Enterprise uses /api whereas GHC uses a sub-domain. We want the
	// base site.
	u, _ := url.Parse(viper.GetString("base_url"))
	u.Host = strings.Replace(u.Host, "api.", "", 1)
	u.Path = path
	return u.String()
}
