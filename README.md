# getJS

Extract JavaScript references from HTML pages, print their URLs, and optionally save external scripts and inline code. This is [bp0lr's fork](https://github.com/bp0lr/getJS) of [003random/getJS](https://github.com/003random/getJS).

## Install

Requires Go 1.26.0 or newer. Use the latest patch of a [supported Go release](https://go.dev/doc/devel/release). The preferred development toolchain is Go 1.27.1; Go can download it automatically with `GOTOOLCHAIN=auto`.

From this checkout:

```sh
go build -o getJS .
./getJS --help
go install .
```

On Windows:

```powershell
go build -o getJS.exe .
.\getJS.exe --help
```

`go install .` installs into your Go binary directory, which must be on PATH. Module changes in the modernization PRs must be merged before remote installation can use them.

## Quick start

Extract absolute references without requesting the scripts:

```sh
getJS --url https://example.com --resolve=false
```

Check script URLs, following page and script redirects:

```sh
getJS --url https://example.com --follow-redirect
```

Print the original HTML attribute values:

```sh
getJS --url https://example.com --complete=false --resolve=false
```

Both `--complete` and `--resolve` default to true. URL completion respects the final page URL after redirects and the first usable HTML `<base href>`.

## Options

| Option | Short | Default | Behavior |
| --- | --- | --- | --- |
| `--url` | `-u` | Empty | Page URL. |
| `--input` | `-i` | Empty | File containing one page URL per line. |
| `--output` | `-o` | Empty | Replace the result file and also print results to stdout. |
| `--header` | `-H` | None | Repeatable custom header for pages and same-origin scripts. |
| `--proxy` | `-p` | Environment | HTTP or HTTPS proxy URL; otherwise standard Go proxy environment variables apply. |
| `--complete` | `-c` | `true` | Print absolute URLs. |
| `--resolve` | `-r` | `true` | Check scripts using GET; retain HTTP 200 or 304. Requires completion. |
| `--save` | `-s` | `false` | Download external scripts and save inline code, including with `--resolve=false`. |
| `--follow-redirect` | `-f` | `false` | Follow redirects, with a maximum chain of 10 requests. |
| `--timeout` | | `15s` | Positive duration for each HTTP request, including body reading. |
| `--insecure` | | `false` | Disable TLS certificate verification explicitly. |
| `--verbose` | `-v` | `false` | Write progress to stderr. |
| `--nocolors` | `-n` | `false` | Compatibility flag; output is always plain text. |
| `--help` | `-h` | | Show help. |

Use `--flag=false` to disable a boolean option. Long options use two hyphens.

## Input and output

Combine stdin, a file, and a single URL:

```sh
cat pages.txt | getJS --url https://example.org --resolve=false
getJS --input pages.txt --output scripts.txt
```

PowerShell:

```powershell
Get-Content pages.txt | getJS --resolve=false
```

Blank input lines are ignored; surrounding whitespace is trimmed. Input lines are limited to 1 MiB. URLs must use HTTP or HTTPS and cannot contain embedded credentials.

Results go to stdout; progress and errors go to stderr. `--output` also prints results. To suppress stdout, redirect to `/dev/null` in a POSIX shell or `$null` in PowerShell.

| Exit code | Meaning |
| --- | --- |
| 0 | Completed successfully, including pages with no scripts. |
| 1 | Operational failure, including an output write failure. |
| 2 | Partial failure: some pages or scripts succeeded and others failed. |
| 3 | Invalid arguments or no input. |
| 130 | Interrupted or cancelled. |

## Headers and downloads

```sh
getJS -u https://example.com -H "Authorization: Bearer example" -H "X-Context: value:with:colons"
getJS -u https://example.com --save --follow-redirect
```

Custom headers are sent to each supplied page and its same-origin scripts. They are removed when a redirect chain changes origin and are not restored later in that chain. Same origin means equal scheme, hostname, and effective port. Header values are not logged.

Downloads go under `download/<page-host>/`. Names are sanitized for Windows and existing files are preserved using numbered suffixes. Incomplete files are removed when a write fails. HTTP 304 is accepted for checks but cannot produce a download without a cached body.

## Scope and limitations

- Reads `src` and `data-src` from script elements. Inline text is saved with `--save`.
- Parses returned HTML only; does not execute JavaScript or observe browser-injected scripts.
- Ignores non-HTTP references for network operations.
- Requires HTTP 200 for HTML and HTTP 200 or 304 for script checks. Status alone does not prove the body is JavaScript.
- Concurrency, deduplication, JSONL, size limits, offline HTML, and additional extraction options are tracked in [PLAN.md](PLAN.md).

## Development

```sh
go test ./...
go vet ./...
go build ./...
```

Tests use local HTTP servers, TLS fixtures, and temporary directories. See [PLAN.md](PLAN.md) for the PR sequence and acceptance criteria.

## Contributing and credits

Use [this fork's issue tracker](https://github.com/bp0lr/getJS/issues). Include a minimal example and remove credentials from logs.

The previous README declared MIT licensing; restoring a standalone license and its original attribution is tracked in the plan.

Thanks to [003random](https://github.com/003random) for the original implementation and [pczajkowski](https://github.com/pczajkowski) for credited improvements and ideas.
