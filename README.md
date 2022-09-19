# Codesearch

GitHub search needs a lot of work.

Ultimately, I want a command-line program with grep-like semantics. I want code
search to be fluid. Iterative. And most interfaces built for web won't enable that.

There are a handful of tools I was spoiled by @facebook and `BigGrep` was one
of them. `cs` usage is modeled after what I remember about it. It wraps around
GitHub's REST and GraphQL APIs. One to get a collection of ranked results, the
other to get file contents for stuff like `--context`.

There are some quirks. We have a default `--limit` of 30. You can increase but
anything above 100 incurs round-trips to GitHub. Rapid fire queries can
sometimes get rate-limite for ~30s. But overall it works fairly well.

Don't hesitate to give feedback if you've got it. :)

![](./demo.gif)

# What about Sourcegraph? LiveGrep?

Listen, I'm barely navigating the sea of SaaS as it is. My company doesn't have
it and this only cost booze and a weekend.

# Install

```
> go install github.com/coxley/codesearch/cs@latest

# Have fun!
> cs --help

Codesearch wraps the GitHub API to be closer to grep/ag/et al semantics

Positional args are merged into a single string and used as the search term. Refer to
GitHub's documentation for nuances: https://docs.github.com/en/search-github/searching-on-github/searching-code

While we've done our best, GitHub can be harsh with ratelimiting. If your org
has consistent branch names, consider running 'cs set-default-branch' to
alleviate some pressure.

Usage:
  cs [terms] [flags]
  cs [command]

Available Commands:
  help                 Help about any command
  set-default-branch   Set default branch name to use when fetching file contents
  set-org              Scope all searches to be within a GitHub organization
  set-token            Set a new personal access token to use for talking to GitHub
  unset-default-branch Revert to default behavior of querying GitHub for precise branch names

Flags:
  -A, --after-context int    print [num] lines of trailing context after each match
  -B, --before-context int   print [num] lines of leading context before each match
      --config string        overrides location of the config file
      --content              print only the text results, nothing else
  -C, --context int          print [num] lines of context before and after each match
  -c, --count                print only a count of matches
      --dump                 dump result structures to stdout
  -x, --ext string           scope search by file extension
  -f, --filename string      scope search by filename
  -l, --files-only           print only filenames of matches to stdout
      --format string        custom format string (variables: $owner, $repo_name, $repo_path, $path, $lineno, $colno, $text, $url) (default "$repo_path:$lineno: $text")
      --full-names-only      print only fully-qualified repo names to stdout (your/repo path/to/README.md)
  -h, --help                 help for cs
      --lang string          scope search with a single language:[lang]
      --limit int            limit the number of matches queried and displayed (default 30)
  -o, --org string           scope search with a single organization:[org]
  -p, --path string          scope search by the path files are in
      --repos-only           print only repository names containing matches to stdout
  -q, --show-query           show the search terms we would send to GitHub and exit
      --tabwidth int         number of spaces to display tabs as (default 2)
  -u, --url                  print URLs to the selected line as the prefix before text
  -v, --verbose              prints verbose messages to stderr for debugging

Use "cs [command] --help" for more information about a command.
```

# Tips & Tricks

**Set a default org**:

While `cs` works for anything on Github, it's best use-case is within a company
/ team / whatever-structure hosting their projects here. That... and you're
likely to find the results you want before being rate-limited this way.

You can change it at any time - even between invocations with `--org/-o`!

```
# cs set-org [name] works too
> cs set-org
? Which one?  [Use arrows to move, type to filter]
  trigger
> TriggerMail
  (enter manually)
```

**URLs**:

Directly click into a highlighted line or email it to a friend.

```
> cs fmt.Fprintf --lang go --url
https://github.com/facebook/time/blob/main/cmd/pshark/main.go#L210:    fmt.Fprintf(flag.CommandLine.Output(), "pshark: PTP-specific poor man's tshark. Dumps PTPv2 packets parsed from capture file to stdout.\nUsage:\n")
https://github.com/facebook/time/blob/main/cmd/pshark/main.go#L210:    fmt.Fprintf(flag.CommandLine.Output(), "pshark: PTP-specific poor man's tshark. Dumps PTPv2 packets parsed from capture file to stdout.\nUsage:\n")
https://github.com/facebook/time/blob/main/cmd/pshark/main.go#L211:    fmt.Fprintf(flag.CommandLine.Output(), "%s [file]\n", os.Args[0])
https://github.com/facebook/time/blob/main/cmd/pshark/main.go#L211:    fmt.Fprintf(flag.CommandLine.Output(), "%s [file]\n", os.Args[0])
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L56:  fmt.Fprintf(w, "N\t")
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L56:  fmt.Fprintf(w, "N\t")
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L58:    fmt.Fprintf(w, "%d \t", i)
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L58:    fmt.Fprintf(w, "%d \t", i)
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L61:  fmt.Fprintf(w, "delay\t")
https://github.com/facebook/time/blob/main/cmd/ptpcheck/cmd/trace.go#L61:  fmt.Fprintf(w, "delay\t")
```

**Context**:

Like grep and siblings, `cs` supports contextual line flags. Leading, trailing,
and surrounding, oh my!

```

> cs user:coxley 'switch key.Key' -A 39
rtprompt/rtprompt.go:175:     switch key.Rune {
rtprompt/rtprompt.go:176:     case 'b':
rtprompt/rtprompt.go:177:       // back a word
rtprompt/rtprompt.go:178:       p.cursorLeft(p.pos - p.lastWordIndex() - 1)
rtprompt/rtprompt.go:179:     case 'f':
rtprompt/rtprompt.go:180:       // forward a word
rtprompt/rtprompt.go:181:       p.print(fmt.Sprintf("next word: %d", p.nextWordIndex()), 4)
rtprompt/rtprompt.go:182:       p.cursorRight(p.nextWordIndex() - p.pos + 1)
rtprompt/rtprompt.go:183:     }
rtprompt/rtprompt.go:184:   case keyboard.KeyArrowLeft, keyboard.KeyCtrlB:
rtprompt/rtprompt.go:185:     p.cursorLeft(1)
rtprompt/rtprompt.go:186:   case keyboard.KeyArrowRight, keyboard.KeyCtrlF:
rtprompt/rtprompt.go:187:     p.cursorRight(1)
rtprompt/rtprompt.go:188:   case keyboard.KeyBackspace, keyboard.KeyBackspace2:
rtprompt/rtprompt.go:189:     p.backspace(1)
rtprompt/rtprompt.go:190:   case keyboard.KeyDelete, keyboard.KeyCtrlD:
rtprompt/rtprompt.go:191:     p.del(1)
rtprompt/rtprompt.go:192:   case keyboard.KeyCtrlA, keyboard.KeyHome:
rtprompt/rtprompt.go:193:     // Beginning of line
rtprompt/rtprompt.go:194:     p.cursorLeft(p.pos)
rtprompt/rtprompt.go:195:   case keyboard.KeyCtrlE, keyboard.KeyEnd:
rtprompt/rtprompt.go:196:     // End of line
rtprompt/rtprompt.go:197:     p.cursorRight(len(p.text) - p.pos)
rtprompt/rtprompt.go:198:   case keyboard.KeyCtrlU:
rtprompt/rtprompt.go:199:     // Remove text before cursor
rtprompt/rtprompt.go:200:     p.backspace(p.pos)
rtprompt/rtprompt.go:201:   case keyboard.KeyCtrlK:
rtprompt/rtprompt.go:202:     // Remove text from cursor to EOL
rtprompt/rtprompt.go:203:     p.del(len(p.text) - p.pos)
rtprompt/rtprompt.go:204:   case keyboard.KeyCtrlW:
rtprompt/rtprompt.go:205:     p.backspace(p.pos - p.lastWordIndex() - 1)
rtprompt/rtprompt.go:206:   case keyboard.KeySpace:
rtprompt/rtprompt.go:207:     p.advance(" ")
rtprompt/rtprompt.go:208:   default:
rtprompt/rtprompt.go:209:     // Do nothing without letter/digit
rtprompt/rtprompt.go:210:     if key.Rune == 0 {
rtprompt/rtprompt.go:211:       break
rtprompt/rtprompt.go:212:     }
rtprompt/rtprompt.go:213:     p.advance(string(key.Rune))
rtprompt/rtprompt.go:214:   }
```
