# getJS

Extract JavaScript references from HTML pages, print their URLs, and optionally save external scripts and inline code.

This is [bp0lr's fork](https://github.com/bp0lr/getJS) of [003random/getJS](https://github.com/003random/getJS). It adds custom request headers and script downloads.

## Requirements and installation

Go 1.26.0 or newer is required. Use the latest patch release of a supported Go branch. This checkout selects Go 1.27.1 as its preferred toolchain and pins dependencies in `go.mod` and `go.sum`.

Build from this checkout:

```sh
go build -o getJS .
./getJS --help
```

On Windows:

```powershell
go build -o getJS.exe .
.\getJS.exe --help
```

To install from the checkout into your Go binary directory, run `go install .` and ensure that directory is on your PATH. Go can download the selected toolchain automatically when `GOTOOLCHAIN=auto`; the first build also needs access to download dependencies.

The minimum version follows Go's [two-release support policy](https://go.dev/doc/devel/release#policy). See [Go downloads](https://go.dev/dl/) for current patches and the [installation guide](https://go.dev/doc/go-get-install-deprecation) for `go install` usage. These module changes must be published before remote installation can use them.

Build and vet have been checked with Go 1.26.0 and Go 1.27.1 on Windows; command help was also checked with Go 1.27.1. `go test ./...` completes on both versions but reports no test files; behavioral tests and the fixes in [PLAN.md](PLAN.md) are still pending.

The examples below assume `getJS` is on your PATH. For a local build, use `./getJS` or `.\getJS.exe` instead.

## Features

- Reads page URLs from `--url`, `--input`, and stdin, combining all supplied sources.
- Extracts `src` and `data-src` attributes from `<script>` elements.
- Completes relative references and checks script URLs by default.
- Prints one matching script URL per line.
- Optionally writes the URL list to a file or downloads scripts and inline code.
- Supports custom page request headers, an HTTP proxy, and optional redirect following.

getJS parses server-returned HTML. It does not execute JavaScript, render pages in a browser, or discover scripts injected only at runtime. Inline code is saved with `--save` but is not included in the printed URL list.

## Quick start

Extract references without requesting each script:

```sh
getJS --url https://example.com --resolve=false
```

Extract references and check whether each script returns HTTP 200 or 304:

```sh
getJS --url https://example.com
```

Both `--complete` and `--resolve` default to `true`. To print the original attribute values, disable both:

```sh
getJS --url https://example.com --complete=false --resolve=false
```

## Options

| Option | Short | Default | Behavior |
| --- | --- | --- | --- |
| `--url` | `-u` | Empty | Page URL to process. |
| `--input` | `-i` | Empty | Text file containing one page URL per line. |
| `--output` | `-o` | Empty | Write the URL list to a file, replacing its contents. Results still print to stdout. |
| `--header` | `-H` | None | Add a header to page requests. Repeat for multiple headers. |
| `--proxy` | `-p` | Empty | HTTP proxy URL. |
| `--complete` | `-c` | `true` | Convert references to full URLs. See the relative URL limitations below. |
| `--resolve` | `-r` | `true` | Request scripts with GET and retain HTTP 200 or 304 responses. Requires `--complete=true`. |
| `--save` | `-s` | `false` | Save inline code; also save external scripts when resolution is enabled. |
| `--follow-redirect` | `-f` | `false` | Follow redirects for page and script requests. |
| `--verbose` | `-v` | `false` | Print progress and additional errors. Progress currently goes to stdout. |
| `--nocolors` | `-n` | `false` | Disable colored log output. |
| `--help` | `-h` | | Show help. |

Use `--flag=false` to disable a boolean option. Use double hyphens for long option names.

## Examples

Read a file:

```sh
getJS --input pages.txt --resolve=false
```

Read a pipeline:

```sh
cat pages.txt | getJS --resolve=false
```

In PowerShell:

```powershell
Get-Content pages.txt | getJS --resolve=false
```

Combine input sources:

```sh
echo https://example.com | getJS --input pages.txt --url https://example.org --resolve=false
```

Add custom headers:

```sh
getJS -u https://example.com -H "Cookie: session=example" -H "User-Agent: MyClient"
```

Follow redirects and use a proxy:

```sh
getJS --url https://example.com --follow-redirect --proxy http://127.0.0.1:8080
```

Save scripts and inline code:

```sh
getJS --url https://example.com --save
```

For this single-URL example, downloads go to `download/example.com/`. External filenames use the basename of the URL path. Inline files start with `inline.js`.

Write the URL list to a file:

```sh
getJS --url https://example.com --output scripts.txt
```

`--output` also prints results. To suppress stdout, redirect it to `/dev/null` in a POSIX shell or `$null` in PowerShell.

## Known limitations

- Requests have no effective timeout, and TLS certificate verification is disabled.
- Requests run sequentially, with a separate HTTP client and transport for each request. Duplicate references are processed repeatedly.
- Relative URL completion mishandles page paths and parent directory references. A reference containing only `/` can cause a panic. HTML `<base href>` and the final page URL after redirects are not used to complete references.
- Headers apply to page requests, but not to script checks or downloads. Values containing additional colons are discarded. Verbose logging can print supplied header values.
- Download grouping uses the host from `--url` for every page. With file or pipeline input alone, files go directly under `download/`.
- Filename collisions can produce names such as `app12.js` and overwrite a file once the retry limit is reached. Directory and write errors are not consistently propagated.
- HTML pages must return HTTP 200. Script checks accept HTTP 200 or 304 without verifying JavaScript content. A 304 response normally has no script body to save.
- Most errors are hidden unless `--verbose` is enabled, some errors appear on stdout, and many failures still return a successful process exit code.
- Input lines are not trimmed or deduplicated. Standard scanner limits apply. Inputs and file output results are accumulated in memory.

See [PLAN.md](PLAN.md) for fixes, priorities, and validation criteria. Planned options are not available yet.

## Contributing

Report bugs and suggestions in [this fork's issue tracker](https://github.com/bp0lr/getJS/issues). Include the command, Go version, expected behavior, and a small HTML example when relevant. Remove credentials from examples and logs.

## License and credits

The previous README declared this project MIT-licensed. This checkout has no standalone license file; restoring the original license text and attribution is tracked in the improvement plan.

- [003random](https://github.com/003random): original implementation.
- [pczajkowski](https://github.com/pczajkowski): improvements and ideas credited by the original README.
