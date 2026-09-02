# 423 — Fix watcher/chat mouse scroll: GotoBottom wipe, Y-routing fudge, viewport init hygiene

## Problem

Reported symptom: mouse wheel scroll appears broken in the chat and watcher
panes, while it works in the incident table. An initial diagnosis blamed an
uninitialized `chatViewport` (`MouseWheelEnabled == false`). A source-level
audit against bubbles v1.0.0 and bubbletea v1.3.10 **refuted that mechanism**
(see "Refuted diagnosis") and confirmed two real defects plus one hygiene item.

| ID | Severity | Summary |
|----|----------|---------|
| B1 | HIGH | `updateWatcherViewport` called `GotoBottom()` unconditionally on every update — every stream chunk and 30ms typewriter tick snapped the watcher back to bottom, so wheel scroll appeared dead while output animated |
| B2 | MEDIUM | Wheel routing to the watcher pane depended on `mouseWatcherStartY()`, which ended in a hardcoded `-4` fudge; measured off-by-2 at 80×40, so wheel events over the watcher scrolled the incident table instead |
| B3 | LOW | `chatViewport` was never constructed with `viewport.New()`; it worked only via bubbles' lazy self-init inside `Update`. Hygiene fix, **not** the scroll bug |

## Refuted diagnosis (do not "re-fix")

The original review claimed the chat pane can't wheel-scroll because
`chatViewport` is a zero-value `viewport.Model` with `MouseWheelEnabled == false`.
That is disproven by bubbles v1.0.0 source: `Update` lazily self-initializes
(`setInitialValues` sets `MouseWheelEnabled=true`, `MouseWheelDelta=3`) **before**
the mouse case runs, so the first forwarded wheel event initializes the viewport
and scrolls it. Verified: mouse reporting enabled (`tea.WithMouseCellMotion`,
`cmd/root.go:320,570`), wheel events carry `Action == MouseActionPress`
(bubbletea v1.3.10), `tui.go:746` dispatches `tea.MouseMsg` to `handleMouseMsg`,
and the `m.chatMode` branch forwards unconditionally. Test T4
(`TestMouseScroll_ChatPaneScrolls`) confirms chat wheel scroll works and passes
against `main`. B3 must not be presented as the fix for the symptom.

## Fixes

### B1 — follow-at-bottom guard in `updateWatcherViewport` (`watcher.go`)

Snapshot `AtBottom()` before `SetContent`, and only `GotoBottom()` if the user
was already at the bottom — mirroring the already-correct `updateChatViewport`.
Streaming still auto-follows when the user hasn't scrolled away; an explicit
scroll-up pins the view.

**Call-site audit:** all 13 `updateWatcherViewport()` callers
(`claude.go`, `tui.go`, `watcher.go`) are streaming/append updates during active
output, where follow-at-bottom is exactly the desired behavior. None needed an
explicit `GotoBottom()`; no call sites in `tui.go`/`claude.go` were changed.

### B2 — measure the render instead of re-deriving it (`views.go`, `mouse.go`)

Extracted `renderMainAboveWatcher()` (`views.go`) — the table container +
footer + trailing newlines that `View()`'s default case writes above the
watcher/approvals pane. Both `View()` and `mouseWatcherStartY()` consume it, so
the wheel-routing boundary can never drift from the actual render.
`mouseWatcherStartY()` now returns
`lineRows(renderHeader()) + lineRows(renderMainAboveWatcher())` (Main adds no
top border/margin/padding, so no extra offset). The `-4` fudge and the
`layoutHeaderLines + …` reconstruction are gone. Wheel events are
low-frequency, so measuring per event is fine; no caching in `View()` (value
receiver discards mutations).

Scope: `approvalsExpanded` renders the approvals pane in the same slot; its
behavior (no wheel scroll) is unchanged.

### B3 — construct `chatViewport` explicitly (`model.go`)

Added `newChatViewport()` (mirrors `newWatcherViewport()`) and set
`chatViewport:` in both `InitialModel` and `InitialModelWithConfig`.
Consistency/hygiene only — no runtime behavior change.

## Tests

- **T1** (`TestWatcherViewport_PreservesScrollOnUpdate`) — B1. Fails on `main`
  (expected YOffset 140, got 150 from the unconditional snap). Two cases:
  scrolled-up position preserved across an update; at-bottom keeps following.
- **T2** (`TestMouseWatcherStartY_MatchesRenderedBoundary`,
  `TestMouseScroll_WatcherBoundaryRouting`) — B2. Non-tautological: locates the
  watcher pane's real first row in the composed `m.View()` string and asserts
  the router boundary equals it; drives real `tea.MouseMsg`s (boundary → scrolls
  watcher; above boundary → moves table). Fails on `main` (formula returns 20,
  render is at 22).
- **T3** (`TestMouseWatcherStartY_LivePin`) — B2. Independent regression pin at
  a fixed 80×40 size tying the boundary to the measured pane row.
- **T4** (`TestMouseScroll_ChatPaneScrolls`) — documents chat wheel scroll works
  end-to-end (passes on `main`; guards the refuted assumption).
- De-tautologized: `TestMouseScroll_WatcherExpandedScrollWatcher` now derives
  its Y from the measured render; `TestMouseWatcherStartY` replaced by T2/T3;
  `TestMouseMsg_WatcherExpanded` given a real Y and a real assertion.

## Verification

- `gofmt -s`, `go vet`, `golangci-lint`, `go test ./...`, and
  `go test -race ./pkg/tui/...` all green.
- Golden tests unchanged — none of these fixes alter rendered output, only
  scroll state and event routing.
- Manual (tui-mcp, 120×40, `--dev`): wheel over the table (row 5) moves the
  table selection; wheel over the watcher pane (row 29, past the fixed
  boundary) leaves the table selection untouched — confirming correct routing.
  On `main` the off-by-2 boundary would have leaked that event into the table.

## Lessons / prevention

- **Tautological tests hide layout drift.** The pre-existing watcher-scroll
  tests generated their expected coordinate by calling the very function under
  test (`mouseWatcherStartY()`), so they could never catch the `-4` fudge
  drifting from the real render. Boundary/coordinate tests must derive the
  expected value from an independent source — here, the composed `View()`
  string. Applies to plan 422's convention of recording fail-against-`main`.
- **Single source of truth for layout offsets.** Any value used both to render
  and to route/hit-test must be computed from the same helper, not
  re-derived from constants with a hand-tuned adjustment.

## Files modified

- `pkg/tui/watcher.go` — B1 guard
- `pkg/tui/views.go` — B2 `renderMainAboveWatcher()` helper + default case
- `pkg/tui/mouse.go` — B2 measured boundary, deleted `-4` formula
- `pkg/tui/model.go` — B3 `newChatViewport()` + both constructor literals
- `pkg/tui/mouse_scroll_test.go`, `pkg/tui/watcher_integration_test.go` — T1–T4,
  de-tautologized rewrites

## Post-mortem

_To be filled after merge (plan-review workflow)._
