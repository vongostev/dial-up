/* |>MANIFEST
@version: 1.1.0
@purpose: Semantic test guide for provider-cleanup policy and the tproxy-health-guardian (nft flush/restore coupling + proxy-mode demote)
@grep_summary: test_guide, qa, classify, definitive, transient, clearprovider, crashfailures, env-fault, guardian, tproxy, flush, reload, demote, portopen, firewall
@structure: ▶ Invariants → Cases → LogMarkers → Commands
@filemap
- F:guide [1][QA test guide] => test_guide
@interactions
- [guide] → references → [ClassifyOutput] (classify.go)
- [guide] → references → [Controller.ClearProvider] (controller.go)
- [guide] → references → [tproxyGuardian.check] (internal/controller/guardian.go)
- [guide] → references → [firewall.ExecManager] (internal/firewall/manager.go)
- [guide] → references → [PortOpen] (netcheck.go)
<|MANIFEST */

/* |>CHANGELOG
[2026-07-10] :: 🚀 :: Added tproxy-health-guardian section (guardian.check flush/restore/demote, firewall ExecManager command construction, PortOpen liveness); removed stale render out-of-scope note (those tests now pass)
[2026-07-07] :: 🚀 :: Initial test guide for provider-cleanup-policy
<|CHANGELOG */

# Test Guide: last_provider.json classified-removal policy

## Invariant

`last_provider.json` is removed in EXACTLY three cases:
1. **Definitive classified crash** — first crash with a captured line matching
   `403 | forbidden | auth | cannot create room | room not found | room invalid | unknown provider`.
2. **Non-definitive crash exceeding maxFailures** — the mutex-guarded safety net,
   keyed on `crashFailures` (NOT `failures`).
3. **Explicit ClearProvider()** — via `/n` or the "Stop" button.

Host-side start faults (config write / pipe / `cmd.Start`) increment `failures`
(backoff) but NEVER `crashFailures`, so they can never remove the file.

## Test cases & expected results

| Case | fake olcrtc / setup | Expected file | Expected provider | crashFailures | failures |
|------|---------------------|---------------|-------------------|---------------|----------|
| Definitive crash | `...status 403 guests cannot create rooms` → exit 1 | **removed** on 1st crash | nil | 0 (reset) | 0 (reset) |
| Transient crash | `429 too many requests` → exit 1 | **kept** | retained | > 0 | > 0 |
| Env start-fault | OlcrtcExe = non-existent binary | **kept** | loaded from file | 0 | > 0 |
| ClearProvider | `/n` or Stop button | **removed** | nil | 0 | 0 |

## Log markers (IMP:9-10) to verify in LDD trace

- `imp:9` `class":"definitive"` `Definitive provider rejection, removing persistence`
  (waitCmd, ExitEval) → file must be gone afterwards.
- `imp:8` `class":"transient"` `Subprocess crashed` with `crashFailures:1` → file kept.
- `imp:8` `Failed to start subprocess` (fork/exec ... no such file) → env fault, crashFailures stays 0.
- `imp:7` `Removed persisted provider` (ClearProvider, ForgetProvider).

## Run commands

```sh
# Classifier unit test
go test -race -v -run TestClassifyOutput ./tests/

# Controller integration (definitive / transient / env-fault / clearprovider)
go test -race -v -run 'TestControllerRemovesProviderOnDefinitiveCrash|TestControllerKeepsProviderOnTransientCrash|TestControllerEnvFailureKeepsProvider|TestClearProviderRemovesFile' ./tests/

# Bot dispatch (Stop → ClearProvider)
go test -race -v -run 'TestDispatchCommands|TestButtonLabelsDispatch' ./tests/

# Full in-scope suite (no data race expected)
go test -race ./...
```

## Out of scope (pre-existing, unrelated)

`TestRenderClient` / `TestRenderServer` (render_test.go) — previously failing on a
trailing-newline YAML mismatch; now passing. No known failing tests in scope.

---

# Test Guide: tproxy-health-guardian

## Invariant

The guardian couples the nft tproxy redirect chain to sing-box liveness so a
sing-box crash can never permanently block LAN internet:

1. **P1B flush** — sing-box dead for `singboxDownThreshold` (10s) AND not already
   flushed → `fw.FlushTproxy(ctx)` empties `singbox_tproxy` (LAN bypasses proxy).
   `flushed` is set to `true` ONLY on success (transient nft failures retry next tick).
2. **P1B restore** — sing-box alive AND `flushed==true` → `fw.ReloadFw4(ctx)`
   rebuilds the ruleset. `flushed` reset to `false` on success; `downSince` reset.
3. **P3 demote** — sing-box alive, route `proxy`, but `PortOpen("127.0.0.1:1080",1s)`
   is false → `sb.SetRoute("direct")` + `onDemote()` (refreshes netCache).
4. **Startup sync** — `Start` calls `fw.TproxyRulesPresent(ctx)` once; `flushed = !present`
   so a bot-restart-after-flush is handled.

All firewall/sing-box errors are logged (IMP:7-8) and swallowed — the watchdog
never crashes. Single goroutine → `downSince`/`flushed` need no mutex.

## Guardian test cases & expected results (white-box: `internal/controller/guardian_test.go`)

| Case | fake sb / setup | Expected |
|------|-----------------|----------|
| Alive + direct | `alive:true, route:"direct"` | flushCnt==0, reloadCnt==0, setCnt==0 |
| Dead, 1st tick | `alive:false`, downSince=zero | downSince set, flushCnt==0, flushed==false |
| Dead, past threshold | `alive:false`, downSince=now-15s | flushCnt==1, flushed==true |
| Recovery after flush | `alive:true`, flushed=true | reloadCnt==1, flushed==false, downSince=zero |
| Proxy + port OPEN | `alive:true, route:"proxy"`, listener | setCnt==0 (no demote) |
| Proxy + port CLOSED | `alive:true, route:"proxy"`, closed port | setCnt==1, setRoute=="direct", onDemote fired |

## Firewall ExecManager cases (`tests/firewall_test.go`)

| Case | fake runner | Expected |
|------|-------------|----------|
| FlushTproxy command | record name+args | `nft flush chain inet fw4 singbox_tproxy` |
| ReloadFw4 command | record name+args | `fw4 reload` |
| TproxyRulesPresent | output w/ "tproxy" / without | true / false |
| Error wrapping | runner returns err | `errors.Is(err, ErrNftFlush)` |

## PortOpen cases (`tests/netcheck_test.go`)

- Open ephemeral listener → `PortOpen(addr, 1s) == true`.
- Bind + close ephemeral port → `PortOpen(addr, 1s) == false` (fast refused).

## Log markers (IMP:7-10) to verify in LDD trace

- `imp:8` `Tproxy chain flushed — LAN traffic now bypasses dead sing-box` (check, P1B).
- `imp:8` `Sing-box down past threshold, flushing tproxy chain` (check, P1B, ATTEMPT).
- `imp:7`/`imp:8` `Sing-box recovered, reloading fw4` / `fw4 reloaded` (check, P1B).
- `imp:8` `olcrtc SOCKS port dead while selector=proxy, demoting to direct` (check, P3).
- `imp:7` `Selector demoted to direct` (check, P3, OK).
- `imp:5` `Guardian goroutine exiting via Stop/ctx` (Start, Watchdog).

## Run commands

```sh
# Guardian white-box logic (flush/restore/demote)
go test -race -v -run 'TestGuardian' ./internal/controller/

# Guardian lifecycle (Start/Stop through Controller) + server-mode no-guardian
go test -race -v -run 'TestGuardian' ./tests/

# Firewall ExecManager command construction + error wrapping
go test -race -v -run 'TestExecManager' ./tests/

# PortOpen liveness
go test -race -v -run 'TestPortOpen' ./tests/

# Full in-scope suite (no data race expected)
go test -race ./...
```

## Out of scope (tproxy guide)

`nft`/`fw4` binaries are absent on dev/CI hosts — `firewall.New` runner fails fast
(logged) in the lifecycle integration test; this is expected and exercised via the
injectable `NewWithRunner` fake in `tests/firewall_test.go`.
