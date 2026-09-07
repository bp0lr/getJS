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

`go install .` installs into your Go binary directory, which must be on PATH.

Install the default branch with:

```sh
go install github.com/bp0lr/getJS@master
```

CI builds binaries for Linux, Windows, and macOS. Download them from successful runs in [GitHub Actions](https://github.com/bp0lr/getJS/actions/workflows/ci.yml).

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
| `--html-file` | | Empty | Local HTML file to parse offline. |
| `--base-url` | | Empty | Required HTTP(S) base URL for local HTML. |
| `--output` | `-o` | Empty | Replace the result file and also print results to stdout. |
| `--header` | `-H` | None | Repeatable custom header for pages and same-origin scripts. |
| `--proxy` | `-p` | Environment | HTTP or HTTPS proxy URL; otherwise standard Go proxy environment variables apply. |
| `--complete` | `-c` | `true` | Print absolute URLs. |
| `--resolve` | `-r` | `true` | Check scripts using GET; retain HTTP 200 or 304. Requires completion. |
| `--save` | `-s` | `false` | Download external scripts and save inline code, including with `--resolve=false`. |
| `--follow-redirect` | `-f` | `false` | Follow redirects, with a maximum chain of 10 requests. |
| `--timeout` | | `15s` | Positive duration for each HTTP request, including body reading. |
| `--concurrency` | | `4` | Maximum simultaneous script tasks, from 1 to 256. |
| `--per-host` | | `2` | Maximum active HTTP requests per hostname, including HTTP/2 streams, from 1 to 256. |
| `--jsonl` | | `false` | One JSON object per discovery or failure, preserving page provenance. |
| `--same-origin` | | `false` | Keep scripts from the final page origin only, before checking or downloading. |
| `--include` | | None | Repeatable Go regexp; keep a URL if any inclusion pattern matches. |
| `--exclude` | | None | Repeatable Go regexp; any exclusion match wins over inclusion. |
| `--output-dir` | | `download` | Download destination directory. |
| `--manifest` | | Empty | Replace a JSONL manifest of completed downloads. Requires `--save`. |
| `--incremental` | | `false` | Reuse verified downloads from the previous manifest. Requires `--save` and `--manifest`. |
| `--max-body-size` | | `10485760` | Maximum HTML or downloaded script bytes (10 MiB by default). |
| `--version` | | | Print version and commit without processing input. |
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

Pages are consumed progressively and duplicate page URLs are skipped. Continuous script workers start new tasks as soon as capacity is available, while output retains document order. Scheduling advances at most four times the worker count beyond the next output position to bound lookahead. Duplicate references share work without occupying extra workers; their metadata remains separate in JSONL.

Plain output contains each absolute resource once; query strings remain significant. Checks are cached across pages, with separate caches for header origins. Downloads are deduplicated within each source page so page directories remain independent. Input and output must be different files.

The HTTP client reuses connections when response bodies permit it. Checks use GET and drain at most 64 KiB; a larger body may require closing the connection. Deduplication and check caches consume memory proportional to unique URLs.

Results go to stdout; progress and errors go to stderr. `--output` also prints results. To suppress stdout, redirect to `/dev/null` in a POSIX shell or `$null` in PowerShell.

| Exit code | Meaning |
| --- | --- |
| 0 | Completed successfully, including pages with no scripts. |
| 1 | Operational failure, including an output write failure. |
| 2 | Partial failure: some pages or scripts succeeded and others failed. |
| 3 | Invalid arguments or no input. |
| 130 | Interrupted or cancelled. |

## Offline HTML

```sh
getJS --html-file page.html --base-url https://example.com/docs/page --jsonl
```

Offline mode makes no HTTP requests. It disables resolution automatically and rejects explicit `--resolve=true`. It cannot be combined with `--url` or `--input`; stdin is ignored. The base URL resolves relative references, and HTML `<base href>` is honored. JSONL records identify the input with an absolute file URI.

With `--save`, offline mode saves inline code only. External script URLs are listed without checks or downloads. Size limits, URL filters, and `--same-origin` still apply. The HTML input and output file must be different.

## Headers and downloads

```sh
getJS -u https://example.com -H "Authorization: Bearer example" -H "X-Context: value:with:colons"
getJS -u https://example.com --save --follow-redirect
```

Custom headers are sent to each supplied page and its same-origin scripts. They are removed when a redirect chain changes origin and are not restored later in that chain. Same origin means equal scheme, hostname, and effective port. Header values are not logged.

`--same-origin` also filters output when checks are disabled. The final page URL defines the origin; an HTML base can resolve references but cannot expand it. Inline code remains included. Page redirects may establish a new page origin; a script redirect leaving that origin is refused before contacting the destination and is reported as a script error.

Select declared script URLs by pattern:

```sh
getJS -u https://example.com --include 'app|worker' --exclude 'vendor' --resolve=false
```

Patterns use Go's regexp syntax and are compiled once. They match the full resolved reference URL, including its query, even with `--complete=false`. Matching happens before script requests; it is not a redirect-destination policy. Inline code has no URL and is unaffected by these patterns.

Downloads go under `<output-dir>/<page-host>/<page-url-hash>/`. External names include a stable URL hash, including query strings. Names are sanitized for Windows and existing files are preserved using numbered suffixes.

Files are written to a temporary path, checked against the size limit, flushed, and published without replacing existing files. Hard links publish complete files atomically where supported. Other filesystems use exclusive creation and copying: the destination is visible during that copy, and reported only after completion. Paths are confined to the selected download root. Temporary files and incomplete fallback copies are cleaned up on handled failures. HTTP 304 is accepted for checks but cannot produce a download without a cached body.

The body limit applies to parsed HTML and saved scripts after automatic HTTP decompression. Checks reject a known oversized Content-Length but may stop after 64 KiB when the length is unknown; they do not certify the full body size. `--max-body-size` accepts an integer from 1 byte to 1 TiB.

## JSONL

```sh
getJS --input pages.txt --jsonl --save --output-dir scripts --output results.jsonl
```

Each object includes `page` and `kind` (`script`, `inline`, `preload`, `modulepreload`, or `page` for a page error). External discoveries include absolute `url` and the original `reference`. Optional fields include `status`, `final_url`, `path`, `size`, and `error`.

Completed downloads also include `sha256`, calculated while writing their content. Incremental runs include `reused: true` when an existing verified file is retained; `status` still describes the actual HTTP response (200 or 304).

JSONL preserves individual discoveries, including repeated references and their source pages, while checks and downloads remain cached. It includes inline records without embedding inline source text. Failed scripts have an error record; failed page fetches have a page error record. Diagnostics still go to stderr and partial failure still returns code 2.

The `attributes` object records `type`, `async`, `defer`, `nomodule`, `integrity`, `crossorigin`, `referrerpolicy`, and link `rel`/`as` when applicable. Boolean HTML attributes use presence semantics: `async="false"` is still present. Empty `crossorigin=""` remains distinguishable from an absent attribute.

`attributes.index` is the one-based position among script and link-with-href candidates in the parsed document, before filtering. Inline filenames use this position. Metadata describes HTML declarations; getJS does not execute scripts or verify Subresource Integrity.

`--complete=false` changes plain-text output only; JSONL always keeps the resolved URL alongside the original reference.

## Download manifest

```sh
getJS --input pages.txt --save --manifest manifest.jsonl --output-dir scripts
```

The manifest contains one object per distinct completed file: `page`, optional external `url`, `kind`, `path`, `size`, and `sha256`. External files also record `final_url` and, when available, `etag`, `last_modified`, and a `request_context` fingerprint. Inline files record their document `index`. Repeated tags that share a cached download produce one manifest entry. Failed downloads produce no entry. Paths are absolute when `--output-dir` is absolute; otherwise they are relative to the process working directory.

Compare hashes to detect content changes or identical content at different URLs. A hash records content identity, not trust or authenticity. The manifest is replaced on each run and must differ from input and result files. An interrupted run can leave a partial manifest; already completed script files remain available.

### Incremental downloads

Run the same command again to update a saved collection:

```sh
getJS --input pages.txt --save --incremental --manifest manifest.jsonl --output-dir scripts
```

A missing manifest starts a new collection. Existing entries are read before any replacement. Files must remain inside the selected output directory and match the recorded size and SHA-256. Missing, changed, or oversized cached files trigger a regular download. Keep the same working directory when using relative paths.

External scripts are always requested. For a verified file, getJS sends `If-None-Match` (ETag), or `If-Modified-Since` when no ETag is available. Validators follow only the previously recorded final URL, so a changed redirect target receives an unconditional request. Changes to configured headers, page origin, proxy, or TLS/origin settings invalidate conditional reuse. The manifest stores a configuration fingerprint, without header values.

HTTP 304 reuses the verified local body; if that body disappears or changes during the request, getJS retries once without validators. A 200 response with identical content retains the old file, even if the server ignored validators. Changed content gets a numbered filename and preserves the previous version. Responses with `Cache-Control: no-store` or `Vary: *` do not enable subsequent conditional reuse. Explicit conditional request headers cannot be combined with `--incremental`.

Inline code is reused by source page, document position, and content hash. Offline mode continues to make no network requests. Older manifests without validators still allow content comparison after a normal download.

Incremental manifests are staged and replace the previous manifest only when the entire run succeeds. A failed or interrupted run preserves the old manifest; files already completed remain on disk. A later run may create another copy of files completed during a failed run because they are absent from that preserved manifest. Invalid manifests cause an error before processing. No separate cache database is created.

## Scope and limitations

- Reads `src` and `data-src` from script elements. Inline text is saved with `--save`.
- Also reads `link[rel~=modulepreload]` with a script-like destination and `link[rel~=preload][as=script]`, in document order. Explicit non-script destinations are excluded. A preload is a declared resource hint, not evidence of execution.
- Parses returned HTML only; does not execute JavaScript or observe browser-injected scripts.
- Ignores non-HTTP references for network operations.
- Requires HTTP 200 for HTML and HTTP 200 or 304 for script checks. Status alone does not prove the body is JavaScript.

## Development

### Implementation sequence for v0.1.0

1. Identify installed versions and local Git builds automatically; prepare the first versioned release.
2. Replace batch barriers with continuous script workers, preserving ordered output and bounded lookahead.
3. Add portable file publication for destinations without hard-link support.
4. Reuse verified downloads with conditional HTTP requests and persistent validators.
5. Run the platform matrix and race detector, verify license attribution, then publish binaries and checksums.

Changes are kept in separate commits. This README is the only Markdown document in the repository.

### Build and test

```sh
go test ./...
go vet ./...
go build ./...
```

Tests use local HTTP servers, TLS fixtures, and temporary directories. CI tests Go 1.26.0 and 1.27.1 on Linux, Windows, and macOS, and runs the race detector on Linux.

Build with version information:

```sh
go build -ldflags="-X main.version=dev -X main.commit=local" .
```

`--version` uses module build information for `go install`, Git revision information for local builds, and explicit linker values when supplied. Local builds with uncommitted changes show `[modified]`. Installed release tags identify the version even when Go does not embed a Git revision.

### Performance measurements

```sh
go test -run '^$' -bench 'Benchmark(Pipeline|CheckMethod)' -benchmem ./...
```

A local synthetic sample on Go 1.27.1, Windows amd64 and Ryzen 9 3900X used a page with 16 distinct 1 KiB scripts, each referenced twice, and a 2 ms response delay. Three repetitions of three iterations produced these median batch times:

| Concurrent tasks | Earlier batch implementation | Continuous workers | Requests |
| --- | --- | --- | --- |
| 1 | 41.62 ms | 42.37 ms | 17 |
| 4 | 21.24 ms | 11.20 ms | 17 |
| 8 | 11.70 ms | 7.31 ms | 17 |

The per-host limit matched concurrency for the experiment; the normal default is 2. These are small samples from two revisions of the same fixture, not Internet speed guarantees. Continuous workers allocated about 279 to 285 KB/op at concurrency 1 and 497 to 521 KB/op at concurrency 8; allocated bytes are not peak memory.

For a separate 1 KiB fixture with 1 ms delay, HEAD with fallback took about 3.06 ms when HEAD returned 405, versus 1.56 ms for GET. GET remains the default. DOM replacement and browser execution are outside this update.

### Compatibility changes

- TLS certificates are verified by default; `--insecure` is explicit.
- Errors are visible on stderr and may return a nonzero exit code.
- Plain output is deduplicated; JSONL preserves discoveries and their origin pages.
- Downloads use portable names and preserve existing files.
- Headers apply to same-origin scripts and are removed on origin changes.
- `--save --resolve=false` still downloads external scripts in online mode.
- `--nocolors` remains accepted; output uses plain text.

## Contributing and credits

Use [this fork's issue tracker](https://github.com/bp0lr/getJS/issues). Include a minimal example and remove credentials from logs.

The original README declares this project MIT-licensed. This repository does not include a separate license file.

Thanks to [003random](https://github.com/003random) for the original implementation and [pczajkowski](https://github.com/pczajkowski) for credited improvements and ideas.
