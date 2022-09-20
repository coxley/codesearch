package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/fatih/color"
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
