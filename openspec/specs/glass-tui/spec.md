# Glass TUI Specification

## Purpose

Glass is the TUI dashboard for Maestro. It provides a live, interactive terminal interface for monitoring and managing containers, images, volumes, networks, and logs. The dashboard is organized into tabbed views named after Dark Tower locations: Mid-World (containers), End-World (images), Dogan (volumes), Beams (networks), and Oracle (logs).

Maerlyn's Rainbow lets the viewer see what is happening across multiple worlds simultaneously.

---

## Requirements

### Requirement: Dashboard Command

The system MUST provide a dashboard command that launches the TUI application. The TUI MUST take over the full terminal and render an interactive interface. The system MUST detect whether the terminal supports TUI rendering and provide a graceful fallback for non-TUI terminals.

#### Scenario: Launch dashboard in capable terminal

GIVEN the terminal supports full-screen TUI rendering
WHEN the user invokes the dashboard command
THEN the TUI MUST take over the full terminal
AND the default tab (Mid-World / containers) MUST be displayed

#### Scenario: Fallback for non-TUI terminal

GIVEN the output is not a TTY (e.g., piped or redirected)
WHEN the user invokes the dashboard command
THEN the system MUST display an error indicating that an interactive terminal is required
AND the system MUST suggest using the standard CLI commands instead

---

### Requirement: Tab Navigation

The system MUST provide tabbed navigation across five views: Mid-World (containers), End-World (images), Dogan (volumes), Beams (networks), and Oracle (logs). The user MUST be able to switch between tabs using keyboard input.

#### Scenario: Switch between tabs

GIVEN the dashboard is running and the Mid-World tab is active
WHEN the user presses the tab navigation key
THEN the next tab MUST become active
AND the view MUST update to show the content of the newly selected tab

#### Scenario: All five tabs accessible

GIVEN the dashboard is running
WHEN the user cycles through all tabs
THEN each of the five views (Mid-World, End-World, Dogan, Beams, Oracle) MUST be reachable
AND each view MUST display data relevant to its domain

---

### Requirement: Keyboard Shortcuts

The system MUST support keyboard shortcuts for common operations within the dashboard. At minimum: Enter (inspect selected resource), d (delete with confirmation), l (view logs for selected container), s (stop selected container), k (kill selected container), and q (quit the dashboard).

#### Scenario: Delete with confirmation prompt

GIVEN the dashboard is running and a container is selected in the Mid-World tab
WHEN the user presses 'd'
THEN a confirmation prompt MUST be displayed
AND the container MUST only be deleted if the user confirms
AND if the user cancels, the dashboard MUST return to the previous view without changes

#### Scenario: Quit dashboard

GIVEN the dashboard is running
WHEN the user presses 'q'
THEN the TUI MUST exit cleanly
AND the terminal MUST be restored to its original state

---

### Requirement: Live Refresh

The system MUST refresh the displayed data at regular intervals to reflect the current state of the system. Resource lists, container statuses, and metrics MUST update without requiring manual user action.

#### Scenario: Container status updates automatically

GIVEN the dashboard is displaying the Mid-World tab with a running container
WHEN the container is stopped via another terminal session
THEN the dashboard MUST reflect the stopped status within the next refresh cycle
AND no manual refresh action MUST be required

---

### Requirement: Oracle Log Viewer

The Oracle tab MUST provide a log viewer that displays container log output. The user MUST be able to select a container and view its logs in a streaming, follow mode. The log viewer SHOULD support text search within the displayed logs.

#### Scenario: View logs for selected container

GIVEN the Oracle tab is active
WHEN the user selects a container
THEN the log output for that container MUST be displayed
AND new log lines MUST appear in real-time as the container produces output

#### Scenario: Search within logs

GIVEN the Oracle tab is displaying logs for a container
WHEN the user invokes the search function
THEN the system SHOULD allow text search within the displayed log content
AND matching text SHOULD be highlighted
