package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/google/go-github/v47/github"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var defaultCfgFile = ".codesearch"

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "set-token",
		Short: "Set a new personal access token to use for talking to GitHub",
		Run: func(cmd *cobra.Command, args []string) {
			viper.Set("token", askForToken())
			err := viper.WriteConfig()
			if err != nil {
				fatalf("couldn't save config: %v", err)
			}
			fmt.Println("Saved")
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "set-org",
		Short: "Scope all searches to be within a GitHub organization",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 {
				viper.Set("org", args[0])
				err := viper.WriteConfig()
				if err != nil {
					fatalf("couldn't save config: %v", err)
				}
				fmt.Println("Saved")
				return
			}

			var answer string
			names, err := listOrgs()
			if err == nil {
				var answer string
				prompt := &survey.Select{
					Message: "Which one?",
					Options: append(names, "(enter manually)"),
				}
				survey.AskOne(prompt, &answer, survey.WithValidator(survey.Required))

				if answer == "(enter manually)" {
					fmt.Print("Ready when you are: ")
					fmt.Scanln(&answer)
				}
				viper.Set("org", answer)

				err := viper.WriteConfig()
				if err != nil {
					fatalf("couldn't save config: %v", err)
				}
				fmt.Println("Saved")
				return
			}

			fmt.Print("What's the org's name?: ")
			fmt.Scanln(&answer)
			viper.Set("org", answer)
			fmt.Println("Saved")
			return
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "set-default-branch",
		Short: "Set default branch name to use when fetching file contents",
		Long: `
Github's API responses force us to query this separately which incurs an extra
network penalty.

If you know that all of the repo's in your search scope use a consistent
default branch, you can skip this step by setting it. (eg: master or main)
		`,
		Run: func(cmd *cobra.Command, args []string) {
			var answer string
			fmt.Print("What branch name would you like to set?: ")
			fmt.Scanln(&answer)

			viper.Set("defaultBranch", answer)
			err := viper.WriteConfig()
			if err != nil {
				fatalf("couldn't save config: %v", err)
			}
			fmt.Println("Saved")
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "unset-default-branch",
		Short: "Revert to default behavior of querying GitHub for precise branch names",
		Long: `
If your org has inconsistent default branch names OR you're using codesearch
across owners, you can unset it here.
		`,
		Run: func(cmd *cobra.Command, args []string) {
			viper.Set("defaultBranch", "")
			err := viper.WriteConfig()
			if err != nil {
				fatalf("couldn't save config: %v", err)
			}
			fmt.Println("Saved")
		},
	})
}

func initConfig() {
	if flags.cfgFile != "" {
		viper.SetConfigFile(flags.cfgFile)
		return
	}

	viper.SetConfigName(defaultCfgFile)
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		setupFlow()
	}
}

func listOrgs() ([]string, error) {
	ctx := context.Background()
	client := github.NewClient(getAuthenticatedHTTP(ctx))
	orgs, _, err := client.Organizations.List(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, org := range orgs {
		names = append(names, org.GetLogin())
	}
	return names, nil
}

func setupFlow() {
	viper.Set("token", askForToken())

	err := viper.SafeWriteConfig()
	if err != nil {
		fatalf("couldn't save config: %v", err)
	}

	fmt.Println()
	fmt.Println(
		color.GreenString("Awesome! You can run"),
		color.New(color.FgGreen, color.Underline).Sprintf("%s set-org", os.Args[0]),
		color.GreenString("if you'd like\nto scope searches to a specific organization."),
	)

	os.Exit(0)
}

func askForToken() string {
	u, _ := url.Parse("https://github.com/settings/tokens/new")
	q := u.Query()
	q.Add("description", "Codesearch")
	q.Add("scopes", "repo,read:user,read:org")
	u.RawQuery = q.Encode()

	color.Blue(`
Welcome to codesearch!

You'll need a GitHub personal token for this to work. Head
on over to create one: %s`, color.YellowString(u.String()))

	color.Blue(`
Set any expiry you want - it stays on your machine. Just
make sure to give it 'repo' scope at minimum. The other
selected ones are supplementary.
	`)

	fmt.Print("Paste token here: ")
	var token string
	fmt.Scanln(&token)
	return token
}
