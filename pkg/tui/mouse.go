package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// mouseWatcherStartY returns the Y coordinate (0-based row) of the watcher
// pane's top-border row — the boundary at/below which wheel events target the
// watcher rather than the incident table.
//
// It is measured from the same composition View() renders, not re-derived from
// layout constants: the header plus renderMainAboveWatcher() (table + footer).
// Because both this function and View() consume renderMainAboveWatcher(), the
// routing boundary can never drift from the actual render — replacing the old
// hardcoded "-4" fudge that silently misrouted when style/layout changed.
//
// The final view is wrapped in m.styles.Main.Render(...); Main adds no top
// border/margin/padding, so there is no extra top offset to add here. Wheel
// events are low-frequency, so measuring per event is cheap; do not cache in
// View() (value receiver discards mutations).
func (m model) mouseWatcherStartY() int {
	return lineRows(m.renderHeader()) + lineRows(m.renderMainAboveWatcher())
}

// lineRows counts the rendered rows in a string that ends with a trailing
// newline (each row is followed by "\n"), which is how renderHeader and
// renderMainAboveWatcher emit their content.
func lineRows(s string) int {
	return strings.Count(s, "\n")
}

// handleMouseMsg routes mouse events to the correct component based on the
// current view mode and the mouse Y position within the window.
//
// For non-table views (incident viewer, log viewer, and any future views),
// mouse events are forwarded unconditionally to the active viewport via the
// focus-mode dispatch so new views get mouse scroll for free.
//
// For the main table view, Y coordinates determine whether the event targets
// the incident table or the watcher pane.
func (m model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return m, nil
	}

	switch {
	case m.chatMode:
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd

	case m.viewingIncident:
		m.incidentViewer, _ = m.incidentViewer.Update(msg)
		return m, nil

	case m.viewingLog:
		m.logViewer, _ = m.logViewer.Update(msg)
		return m, nil

	case m.configMode, m.bulkSilenceMode, m.teamSelectMode, m.clusterSelectMode, m.mergeMode:
		return m, nil

	default:
		return m.handleMouseScrollMainView(msg)
	}
}

// handleMouseScrollMainView routes wheel events on the main table+watcher screen.
func (m model) handleMouseScrollMainView(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.watcherExpanded && msg.Y >= m.mouseWatcherStartY() {
		var cmd tea.Cmd
		m.watcherViewport, cmd = m.watcherViewport.Update(msg)
		return m, cmd
	}

	switch msg.Button {
	case tea.MouseButtonWheelDown:
		m.table.MoveDown(1)
	case tea.MouseButtonWheelUp:
		m.table.MoveUp(1)
	}
	cmd := m.syncSelectedIncidentToHighlightedRow()
	return m, cmd
}
