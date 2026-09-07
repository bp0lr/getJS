# Local performance measurements

Measured on 2026-09-07 with Go 1.27.1, Windows amd64, AMD Ryzen 9 3900X. These are small synthetic samples, not Internet throughput estimates or a comparison against the historical unmodified program.

## Pipeline

Fixture: one HTML page with 16 distinct scripts, each referenced twice. Every script returns 1 KiB after a 2 ms server delay. Each iteration creates an application instance. The per-host limit equals concurrency for this comparison.

Command:

```sh
go test -run '^$' -bench BenchmarkPipeline -benchtime=3x -count=3 -benchmem ./...
```

| Concurrency | Median time per batch | Requests per batch | Allocation range |
| --- | --- | --- | --- |
| 1 | 41.62 ms | 17 | 253120 to 259418 B/op |
| 4 | 21.24 ms | 17 | 277512 to 283133 B/op |
| 8 | 11.70 ms | 17 | 322192 to 346885 B/op |

The sequential baseline uses the same corrected implementation at concurrency 1. Eight tasks took about 3.6 times less elapsed time in this fixture. Repeated references share checks, but duplicate tasks can occupy batch slots while waiting for a shared result. Allocations increase with concurrency. B/op is allocated memory, not peak resident memory.

Functional tests separately verify output order, exactly two page requests plus eight unique script requests across repeated pages, reuse of at most two connections, and the per-host request limit. Pages remain sequential; concurrency applies to their script tasks.

## HEAD versus GET

Fixture: 1 KiB script with a 1 ms server delay. Compare GET with HEAD, using GET fallback when HEAD returns 405.

```sh
go test -run '^$' -bench BenchmarkCheckMethod -benchtime=3x -count=3 -benchmem ./...
```

| Server behavior | GET median | HEAD with fallback median |
| --- | --- | --- |
| HEAD supported | 1.52 ms | 1.61 ms |
| HEAD returns 405 | 1.56 ms | 3.06 ms |

Keep GET as the default. This sample does not establish a universal benefit from HEAD and shows the extra round trip when fallback is necessary. Large bodies and real network conditions can produce different results.

## Rechecking

Use longer runs and representative HTML/script sizes before tuning production defaults. Current defaults are concurrency 4 and per-host 2. A tokenizer replacement is deferred: these measurements do not demonstrate that DOM parsing is the bottleneck.
