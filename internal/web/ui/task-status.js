// Shared task-status styling — single source of truth for the 8 canonical
// statuses defined in internal/task/status.go. Every view (tasks.js,
// overview.js, manifests.js linked-tasks list) reads from here so a
// status renders identically across tabs.
//
// If you add a status server-side (status.go), you MUST add the matching
// color + icon + label + css class here, or the UI will silently render
// the new state as grey/unknown.
(function(OL) {
  'use strict';

  // Color mapping uses CSS custom properties from style.css so dark/light
  // theme switches stay consistent. Paused shares running's green dot but
  // adds a pause icon to distinguish it without pulling a separate color.
  OL.TASK_STATUS_COLORS = {
    pending:   'var(--text-muted)',
    waiting:   'var(--accent)',
    scheduled: 'var(--yellow)',
    running:   'var(--green)',
    paused:    'var(--yellow)',
    completed: 'var(--green)',
    failed:    'var(--red)',
    cancelled: 'var(--text-muted)',
  };

  // HTML-entity icons so we don't need an icon font. Glyphs picked for
  // semantic clarity at 11-12px; verified on macOS + iOS emoji stacks.
  OL.TASK_STATUS_ICONS = {
    pending:   '\u25CB', // white circle
    waiting:   '\u23F3', // hourglass
    scheduled: '\u23F0', // alarm clock
    running:   '\u25CF', // filled circle
    paused:    '\u23F8', // pause bars
    completed: '\u2713', // checkmark
    failed:    '\u2717', // cross
    cancelled: '\u2015', // horizontal bar
  };

  // Human labels for the CAPS-display convention used across the dashboard.
  OL.TASK_STATUS_LABELS = {
    pending:   'PENDING',
    waiting:   'WAITING',
    scheduled: 'SCHEDULED',
    running:   'RUNNING',
    paused:    'PAUSED',
    completed: 'COMPLETED',
    failed:    'FAILED',
    cancelled: 'CANCELLED',
  };

  // CSS class per status — used by the list renderer for row-level
  // backgrounds. Classes are defined in style.css; adding a status here
  // without a matching CSS rule falls back to the default row style.
  OL.TASK_STATUS_CLASS = {
    pending:   'status-pending',
    waiting:   'status-waiting',
    scheduled: 'status-scheduled',
    running:   'status-running',
    paused:    'status-paused',
    completed: 'status-completed',
    failed:    'status-failed',
    cancelled: 'status-cancelled',
  };

  // Animation hint: whether a status should pulse its indicator to draw
  // the eye. Running alone qualifies; everything else is static.
  OL.TASK_STATUS_PULSE = {
    running: true,
  };

  // Helper: returns the full render trio for a status in one shot so
  // views don't have to look up three maps. Unknown statuses fall back
  // to neutral grey so a server-side addition without a client update
  // degrades legibly rather than disappearing.
  OL.getStatusRender = function(status) {
    return {
      color: OL.TASK_STATUS_COLORS[status] || 'var(--text-muted)',
      icon:  OL.TASK_STATUS_ICONS[status]  || '?',
      label: OL.TASK_STATUS_LABELS[status] || (status ? status.toUpperCase() : 'UNKNOWN'),
      cssClass: OL.TASK_STATUS_CLASS[status] || 'status-unknown',
      pulse: !!OL.TASK_STATUS_PULSE[status],
    };
  };
})(window.OL = window.OL || {});
