# Codesearch

GitHub search needs a lot of work.

Ultimately, you want a command-line with grep-like semantics. It should be
fluid, iterative. Maybe even impress your co-workers.

Codesearch is simple: `cs [term]`. It shows you the information you need
without much more. There are two output modes: the default is similar to
`ripgrep` and `-G` gets you `grep`.

On top of that, we use the [ANSI Hyperlink
Sequence](https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda#the-escape-sequence)
to make everything clickable. Line numbers take you directly to the file with
it highlighted and file names to the file. We're not quite sure where
repository names take you yet but you can find out!

.[](./demo.gif)

# Install

```
go install github.com/coxley/codesearch/cs@latest

# or with a version like...
# go install github.com/coxley/codesearch/cs@v1.0.1
```

# Motivation

There were a handful of tools I was spoiled by when I worked at Facebook.
`BigGrep` was one of them.

`cs` is modeled after it's experience. It wraps
around GitHub's REST and GraphQL APIs. One to get a collection of ranked
results, the other to get file contents for stuff like `--context`, `--before`,
and `--after`.

Just enough color is used to find what you need without being over-bearing.
We're not trying to boil the ocean. We just want to find things.

# What about Sourcegraph? LiveGrep?

Listen, I'm barely navigating the sea of SaaS as it is. My company doesn't have
it and this only cost booze and a weekend.

In all seriousness, I've heard rad things about Sourcegraph. There might even
be opportunity to support it as an alternative backend in `cs`. But you should
be able to enjoy comfortabl search without it.

# Usage

**Setup**:

You should get a setup prompt the first time you run `cs`. If you get bored and
miss it, just delete `~/.codesearch.yaml` and go at it again.

Note: If you're using GitHub Enterprise, the Base URL will be
`https://[domain]/`. We'll take care of adding the api-specific details.


```
> ./cs viper.WriteConfig
Welcome to codesearch!

Not using GitHub Enterprise? Just press enter!
Base URL [https://api.github.com/]:

You'll need a GitHub personal token for this to work. Head
on over to create one: https://github.com/settings/tokens/new?description=Codesearch&scopes=repo%2Cread%3Auser%2Cread%3Aorg

Set any expiry you want - it stays on your machine. Just make sure to give it
'repo' scope at minimum. The other two are supplementary but make everything
smooth.

Paste token here: nicetry

Awesome! You can run ./cs set-org if you'd like
to scope searches to a specific organization.
```

**Set a default org**:

While `cs` works for anything on Github, it's best use-case is within a company
/ team / whatever-structure hosting their projects here. That... and you're
likely to find the results you want before being rate-limited this way.

It's easy to change on the fly so don't feel committed.

```
# cs set-org [name] works too
> cs set-org
? Which one?  [Use arrows to move, type to filter]gt
  trigger
> TriggerMail
  (enter manually)
```

**Searching**:

Your first search - exciting! If you're using a modern terminal, try clicking
on the line numbers, filenames, and repositories.

```
> cs set-org coxley
Saved

# While GitHub requires "owner/repo" for the repo filter, we fix that for you
# if org is set. (regardless of config or --org)
> cs -r codesearch viper
coxley/codesearch:cs/config.go (master)
15:   "github.com/spf13/viper"
25:     viper.SetConfigFile(flags.cfgFile)
29:   viper.SetConfigName(defaultCfgFile)

coxley/codesearch:cs/go.mod (master)
10:   github.com/spf13/viper v1.13.0

coxley/codesearch:cs/go.sum (master)
194: github.com/spf13/viper v1.13.0 h1:BWSJ/M+f+3nmdz9bxB+bWX28kkALN2ok11D0rSo8EJU=
195: github.com/spf13/viper v1.13.0/go.mod h1:Icm2xNL3/8uyh/wFuB1jI7TiTNKp8632Nwegu+zgdYw=

coxley/codesearch:cs/gql.go (master)
22:   "github.com/spf13/viper"
26:   baseURL := viper.Get("base_url").(string)

coxley/codesearch:cs/main.go (master)
24:   "github.com/spf13/viper"
113:   viper.BindPFlag("org", rootCmd.Flags().Lookup("org"))
114:   viper.BindPFlag("format", rootCmd.Flags().Lookup("format"))

coxley/codesearch:cs/utils.go (master)
13:   "github.com/spf13/viper"
48:   baseURL := viper.GetString("base_url")
```

**URLs**:

Sometimes you want the URLs in your face. Use `--url-prefix/-u` for those occassions.

```
> cs -r codesearch viper -u --limit 5
https://github.com/coxley/codesearch/blob/master/cs/config.go#L15 (master)
15:   "github.com/spf13/viper"
25:     viper.SetConfigFile(flags.cfgFile)
29:   viper.SetConfigName(defaultCfgFile)

https://github.com/coxley/codesearch/blob/master/cs/go.mod#L10 (master)
10:   github.com/spf13/viper v1.13.0

https://github.com/coxley/codesearch/blob/master/cs/gql.go#L22 (master)
22:   "github.com/spf13/viper"
26:   baseURL := viper.Get("base_url").(string)
```

**Greppable**:

If you prefer a more retro style, `--greppable/-G` has you covered.

```
> cs -r codesearch viper -u --limit 5 -G
$url:   "github.com/spf13/viper"
$url:     viper.SetConfigFile(flags.cfgFile)
$url:   viper.SetConfigName(defaultCfgFile)
$url:   github.com/spf13/viper v1.13.0
$url:   "github.com/spf13/viper"
$url:   baseURL := viper.Get("base_url").(string)
```

**Context**:

Like grep, ripgrep, and similar, `cs` supports contextual line flags. Leading, trailing,
and surrounding, oh my!

```
> cs -r codesearch StaticTokenSource -A2
coxley/codesearch:cs/utils.go (master)
41:   ts := oauth2.StaticTokenSource(
42:     &oauth2.Token{AccessToken: token},
43:   )
```

**Only Repos**:

Sometimes you only want the repos that match. These are clickable too!

```
> cs spf13/cobra --repos-only
coxley/pmlproxy
coxley/codesearch
```
