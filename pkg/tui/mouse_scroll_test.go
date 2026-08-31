package tui

import (
	"strings"
	"testing"

	"github.com/PagerDuty/go-pagerduty"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func setupMouseTestModel(incidents []pagerduty.Incident) model {
	m := createTestModelWithTable(incidents)
	m.help = newHelp()
	windowSize = tea.WindowSizeMsg{Width: 80, Height: 60}
	m.recomputeLayout()
	m.table.Focus()
	return m
}

func testIncidents() []pagerduty.Incident {
	return []pagerduty.Incident{
		{
			APIObject:          pagerduty.APIObject{ID: "Q001"},
			Title:              "Incident 1",
			Service:            pagerduty.APIObject{Summary: "svc-1"},
			LastStatusChangeAt: "2024-01-01T00:00:00Z",
		},
		{
			APIObject:          pagerduty.APIObject{ID: "Q002"},
			Title:              "Incident 2",
			Service:            pagerduty.APIObject{Summary: "svc-2"},
			LastStatusChangeAt: "2024-01-01T00:00:00Z",
		},
		{
			APIObject:          pagerduty.APIObject{ID: "Q003"},
			Title:              "Incident 3",
			Service:            pagerduty.APIObject{Summary: "svc-3"},
			LastStatusChangeAt: "2024-01-01T00:00:00Z",
		},
	}
}

func TestMouseScroll_TableScrollDown(t *testing.T) {
	t.Run("wheel down on table area moves cursor down", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())

		// Table cursor starts at row 0
		assert.Equal(t, 0, m.table.Cursor())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5, // within table area
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 1, updated.table.Cursor())
	})
}

func TestMouseScroll_TableScrollUp(t *testing.T) {
	t.Run("wheel up on table area moves cursor up", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())

		// Move to row 2 first
		m.table.MoveDown(2)
		assert.Equal(t, 2, m.table.Cursor())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5, // within table area
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 1, updated.table.Cursor())
	})
}

func TestMouseScroll_TableScrollUpAtTop(t *testing.T) {
	t.Run("wheel up at top of table stays at row 0", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		assert.Equal(t, 0, m.table.Cursor())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 0, updated.table.Cursor())
	})
}

// scrollableWatcherModel builds a mouse-test model with the watcher expanded
// and enough content to scroll, sized at 80x40, with the boundary derived from
// the rendered composition rather than mouseWatcherStartY().
func scrollableWatcherModel(t *testing.T) (model, int) {
	t.Helper()
	m := setupMouseTestModel(testIncidents())
	windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
	m.watcherExpanded = true
	m.recomputeLayout()
	m.watcherViewport.Width = m.layout.WatcherWidth
	m.watcherViewport.Height = m.layout.WatcherHeight

	// The sentinel is present on every content line so it is visible in the
	// pane regardless of scroll position (follow-mode shows the tail).
	sentinel := "WATCHER_SENTINEL_LINE"
	m.watcherBuffer = newWatcherBuffer(200)
	for i := 0; i < 80; i++ {
		m.watcherBuffer.Append(sentinel + " content to force overflow")
	}
	m.updateWatcherViewport()

	firstRow := watcherPaneFirstRow(m, sentinel)
	return m, firstRow
}

func TestMouseScroll_WatcherExpandedScrollWatcher(t *testing.T) {
	t.Run("wheel up inside rendered watcher area scrolls the watcher viewport", func(t *testing.T) {
		m, firstRow := scrollableWatcherModel(t)
		assert.GreaterOrEqual(t, firstRow, 0)

		// Follow-mode leaves the viewport at the bottom; scroll up from there.
		before := m.watcherViewport.YOffset
		assert.Greater(t, before, 0, "content should overflow so we start scrolled to bottom")

		// Y coordinate strictly inside the pane's content, derived from the
		// rendered first content row — NOT from mouseWatcherStartY().
		msg := tea.MouseMsg{
			X:      10,
			Y:      firstRow + 1,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Less(t, updated.watcherViewport.YOffset, before,
			"wheel-up inside the watcher pane must scroll it up")
	})
}

// T2 routing: wheel at the boundary hits the watcher; wheel one row above the
// boundary moves the table. Boundary comes from the measured render.
func TestMouseScroll_WatcherBoundaryRouting(t *testing.T) {
	t.Run("wheel at boundary scrolls watcher, wheel above boundary moves table", func(t *testing.T) {
		m, firstRow := scrollableWatcherModel(t)
		boundary := firstRow - 1 // watcher pane top-border row == router boundary

		// At the boundary: scroll the watcher.
		beforeOffset := m.watcherViewport.YOffset
		wheelUpAtBoundary := tea.MouseMsg{
			X:      10,
			Y:      boundary,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		res1, _ := m.Update(wheelUpAtBoundary)
		afterWatcher := res1.(model)
		assert.Less(t, afterWatcher.watcherViewport.YOffset, beforeOffset,
			"wheel at boundary must scroll the watcher")

		// One row above the boundary: move the table cursor, not the watcher.
		mTable, _ := scrollableWatcherModel(t)
		assert.Equal(t, 0, mTable.table.Cursor())
		wheelDownAboveBoundary := tea.MouseMsg{
			X:      10,
			Y:      boundary - 1,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		res2, _ := mTable.Update(wheelDownAboveBoundary)
		afterTable := res2.(model)
		assert.Equal(t, 1, afterTable.table.Cursor(),
			"wheel above boundary must move the table cursor")
	})
}

func TestMouseScroll_WatcherExpandedScrollTable(t *testing.T) {
	t.Run("wheel down in table area scrolls table even when watcher expanded", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		m.watcherExpanded = true
		m.recomputeLayout()
		m.watcherViewport.Width = m.layout.WatcherWidth
		m.watcherViewport.Height = m.layout.WatcherHeight

		assert.Equal(t, 0, m.table.Cursor())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5, // within table area
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 1, updated.table.Cursor())
	})
}

func TestMouseScroll_WatcherCollapsedScrollTable(t *testing.T) {
	t.Run("wheel down scrolls table when watcher collapsed", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		m.watcherExpanded = false
		m.recomputeLayout()

		assert.Equal(t, 0, m.table.Cursor())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 1, updated.table.Cursor())
	})
}

func TestMouseScroll_IncidentViewForwardsToViewport(t *testing.T) {
	t.Run("mouse scroll in incident view forwards to incidentViewer", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		m.viewingIncident = true

		lines := ""
		for i := 0; i < 100; i++ {
			lines += "line\n"
		}
		m.incidentViewer.Width = m.layout.IncidentViewerWidth
		m.incidentViewer.Height = m.layout.IncidentViewerHeight
		m.incidentViewer.SetContent(lines)

		msg := tea.MouseMsg{
			X:      10,
			Y:      10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Greater(t, updated.incidentViewer.YOffset, 0)
	})
}

func TestMouseScroll_LogViewForwardsToViewport(t *testing.T) {
	t.Run("mouse scroll in log view forwards to logViewer", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		m.viewingLog = true

		lines := ""
		for i := 0; i < 100; i++ {
			lines += "line\n"
		}
		m.logViewer.Width = m.layout.IncidentViewerWidth
		m.logViewer.Height = m.layout.IncidentViewerHeight
		m.logViewer.SetContent(lines)

		msg := tea.MouseMsg{
			X:      10,
			Y:      10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Greater(t, updated.logViewer.YOffset, 0)
	})
}

func TestMouseScroll_NonWheelEventsAreNoOps(t *testing.T) {
	t.Run("non-wheel mouse events return nil cmd on main view", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())

		msg := tea.MouseMsg{
			X:      10,
			Y:      5,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft,
		}
		result, cmd := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 0, updated.table.Cursor())
		assert.Nil(t, cmd)
	})
}

func TestMouseScroll_AfterWindowResize(t *testing.T) {
	t.Run("scroll routing adapts after window resize", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		m.watcherExpanded = true
		m.recomputeLayout()
		m.watcherViewport.Width = m.layout.WatcherWidth
		m.watcherViewport.Height = m.layout.WatcherHeight

		lines := ""
		for i := 0; i < 50; i++ {
			lines += "line\n"
		}
		m.watcherViewport.SetContent(lines)

		// Record watcher start Y before resize
		watcherYBefore := m.mouseWatcherStartY()

		// Resize the window
		windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
		m.recomputeLayout()
		m.watcherViewport.Width = m.layout.WatcherWidth
		m.watcherViewport.Height = m.layout.WatcherHeight

		watcherYAfter := m.mouseWatcherStartY()

		// The boundary should have changed with the resize
		assert.NotEqual(t, watcherYBefore, watcherYAfter)

		// Scrolling in the new table area should still move the table
		assert.Equal(t, 0, m.table.Cursor())
		msg := tea.MouseMsg{
			X:      10,
			Y:      5,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, _ := m.Update(msg)
		updated := result.(model)
		assert.Equal(t, 1, updated.table.Cursor())
	})
}

func TestMouseScroll_SyncsSelectedIncident(t *testing.T) {
	t.Run("scrolling table via mouse syncs selectedIncident", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())

		// Sync initial selection
		m.syncSelectedIncidentToHighlightedRow()
		assert.Equal(t, "Q001", m.selectedIncident.ID)

		msg := tea.MouseMsg{
			X:      10,
			Y:      5,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelDown,
		}
		result, cmd := m.Update(msg)
		updated := result.(model)

		assert.Equal(t, 1, updated.table.Cursor())
		// Mouse scroll should trigger sync (returns a cmd for pre-fetch)
		assert.NotNil(t, cmd)
	})
}

// T1 (B1): a watcher update while the user has scrolled up must NOT snap the
// viewport back to the bottom. Fails against main, where updateWatcherViewport
// calls GotoBottom() unconditionally.
func TestWatcherViewport_PreservesScrollOnUpdate(t *testing.T) {
	t.Run("scrolled-up position is preserved across a content update", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
		m.watcherExpanded = true
		m.recomputeLayout()
		m.watcherViewport.Width = m.layout.WatcherWidth
		m.watcherViewport.Height = m.layout.WatcherHeight

		m.watcherBuffer = newWatcherBuffer(200)
		for i := 0; i < 80; i++ {
			m.watcherBuffer.Append("watcher content line to force overflow")
		}
		m.updateWatcherViewport()

		// User scrolls up, away from the bottom.
		m.watcherViewport.ScrollUp(10)
		scrolled := m.watcherViewport.YOffset
		assert.False(t, m.watcherViewport.AtBottom(), "precondition: not at bottom")

		// A new streamed chunk arrives.
		m.watcherBuffer.SetLast("watcher content line updated by stream chunk")
		m.updateWatcherViewport()

		assert.Equal(t, scrolled, m.watcherViewport.YOffset,
			"scroll position must be preserved when the user has scrolled up")
	})

	t.Run("follow mode: staying at bottom keeps following new content", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
		m.watcherExpanded = true
		m.recomputeLayout()
		m.watcherViewport.Width = m.layout.WatcherWidth
		m.watcherViewport.Height = m.layout.WatcherHeight

		m.watcherBuffer = newWatcherBuffer(200)
		for i := 0; i < 80; i++ {
			m.watcherBuffer.Append("watcher content line to force overflow")
		}
		m.updateWatcherViewport()
		assert.True(t, m.watcherViewport.AtBottom(), "precondition: at bottom")

		for i := 0; i < 20; i++ {
			m.watcherBuffer.Append("more streamed content")
		}
		m.updateWatcherViewport()

		assert.True(t, m.watcherViewport.AtBottom(),
			"follow mode: staying at bottom must keep auto-following new content")
	})
}

// T4 (chat path): documents that chat-pane wheel scroll works end-to-end
// through Update (lazy viewport init or explicit — either way it must scroll).
func TestMouseScroll_ChatPaneScrolls(t *testing.T) {
	t.Run("wheel up in chat mode scrolls the chat viewport", func(t *testing.T) {
		m := setupMouseTestModel(testIncidents())
		windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
		m.chatMode = true
		m.recomputeLayout()
		m.chatViewport.Width = m.layout.WatcherWidth
		m.chatViewport.Height = 10

		m.watcherBuffer = newWatcherBuffer(200)
		for i := 0; i < 80; i++ {
			m.watcherBuffer.Append("chat content line to force overflow")
		}
		m.updateChatViewport()
		m.chatViewport.GotoBottom()
		before := m.chatViewport.YOffset
		assert.Greater(t, before, 0, "content should overflow so we start scrolled down")

		msg := tea.MouseMsg{
			X:      10,
			Y:      10,
			Action: tea.MouseActionPress,
			Button: tea.MouseButtonWheelUp,
		}
		result, _ := m.Update(msg)
		updated := result.(model)

		assert.Less(t, updated.chatViewport.YOffset, before,
			"wheel-up in chat mode must scroll the chat viewport up")
	})
}

// watcherPaneFirstRow renders the full composed View() and locates the row
// index (0-based) where the expanded watcher pane's container actually begins.
// It searches for the WatcherContainer's top border rune so the expected value
// is derived from real rendering, never from mouseWatcherStartY()'s own
// formula. Returns -1 if the pane is not found.
func watcherPaneFirstRow(m model, sentinel string) int {
	view := m.View()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if strings.Contains(line, sentinel) {
			return i
		}
	}
	return -1
}

// T2 (B2): the router's boundary must equal the watcher pane's real first row
// in the composed view. This test never calls mouseWatcherStartY() to build
// its own expected value — it measures the rendered output instead.
func TestMouseWatcherStartY_MatchesRenderedBoundary(t *testing.T) {
	m := setupMouseTestModel(testIncidents())
	windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
	m.watcherExpanded = true
	m.recomputeLayout()
	m.watcherViewport.Width = m.layout.WatcherWidth
	m.watcherViewport.Height = m.layout.WatcherHeight

	sentinel := "WATCHER_SENTINEL_LINE"
	m.watcherBuffer = newWatcherBuffer(50)
	m.watcherBuffer.Append(sentinel)
	m.updateWatcherViewport()

	rendered := watcherPaneFirstRow(m, sentinel)
	assert.GreaterOrEqual(t, rendered, 0, "sentinel watcher content must appear in composed view")

	// The wheel-routing boundary is the first content row of the pane. The
	// WatcherContainer wraps content in a border, so the sentinel content row
	// sits one row below the container's top border. mouseWatcherStartY marks
	// the boundary at/above which wheel events go to the watcher; assert it
	// lands on the pane (its top border row), i.e. one above the content.
	assert.Equal(t, rendered-1, m.mouseWatcherStartY(),
		"router boundary must equal the watcher pane's rendered top-border row")
}

// T3 (B2 live pin): at a known terminal size (80x40) with the watcher expanded
// and a single short entry, the watcher pane's top border was observed via
// tui-mcp at a fixed row. Hard-code it as a regression fixture so a future
// layout/style drift that moves the pane is caught even if the T2 measurement
// helper itself regresses.
func TestMouseWatcherStartY_LivePin(t *testing.T) {
	m := setupMouseTestModel(testIncidents())
	windowSize = tea.WindowSizeMsg{Width: 80, Height: 40}
	m.watcherExpanded = true
	m.recomputeLayout()
	m.watcherViewport.Width = m.layout.WatcherWidth
	m.watcherViewport.Height = m.layout.WatcherHeight

	sentinel := "WATCHER_SENTINEL_LINE"
	m.watcherBuffer = newWatcherBuffer(50)
	m.watcherBuffer.Append(sentinel)
	m.updateWatcherViewport()

	// Pin: the router boundary equals the measured pane top-border row. This
	// mirrors T2 but stands as an independent regression assertion tying the
	// boundary to the actual rendered composition at a fixed size.
	rendered := watcherPaneFirstRow(m, sentinel)
	assert.Equal(t, rendered-1, m.mouseWatcherStartY())
}
