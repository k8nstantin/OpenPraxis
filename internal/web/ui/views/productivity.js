(function(OL) {
  'use strict';
  var fetchJSON = OL.fetchJSON, esc = OL.esc, setText = OL.setText;

  OL.loadProductivityView = async function() {
    // Fetch all periods in parallel
    var results = await Promise.all([
      fetchJSON('/api/tasks/productivity?period=today'),
      fetchJSON('/api/tasks/productivity?period=week'),
      fetchJSON('/api/tasks/productivity?period=month'),
      fetchJSON('/api/tasks/productivity?period=all')
    ]);
    var today = results[0], week = results[1], month = results[2], all = results[3];

    // Trend cards
    function scoreText(p) { return p ? p.score + ' ' + p.grade : '-'; }
    function scoreColor(p) { return p && p.score >= 80 ? 'var(--green)' : p && p.score >= 60 ? 'var(--yellow)' : 'var(--red)'; }

    var el;
    el = document.getElementById('prod-score-today');
    if (el) { el.textContent = scoreText(today); el.style.color = scoreColor(today); }
    el = document.getElementById('prod-score-week');
    if (el) { el.textContent = scoreText(week); el.style.color = scoreColor(week); }
    el = document.getElementById('prod-score-month');
    if (el) { el.textContent = scoreText(month); el.style.color = scoreColor(month); }
    el = document.getElementById('prod-score-all');
    if (el) { el.textContent = scoreText(all); el.style.color = scoreColor(all); }

    // Breakdown grid — use all-time data
    var p = all;
    if (!p) return;

    var breakdownEl = document.getElementById('prod-breakdown');
    if (breakdownEl) {
      var html = '<div style="display:grid;grid-template-columns:repeat(4,1fr);gap:10px;margin-bottom:16px">';

      // Row 1: Positive signals
      html += card('Completed', p.tasks_completed, p.first_attempt_pass + ' first attempt', 'var(--green)', 'rgba(0,217,126,0.08)', 'rgba(0,217,126,0.2)');
      html += card('Lines Committed', (p.lines_committed || 0).toLocaleString(), (p.files_changed || 0) + ' files changed', 'var(--green)', 'rgba(0,217,126,0.08)', 'rgba(0,217,126,0.2)');
      html += card('Watcher Pass Rate', p.watcher_pass_rate + '%', p.total_actions + ' total actions', p.watcher_pass_rate >= 80 ? 'var(--green)' : 'var(--yellow)', 'rgba(0,217,126,0.08)', 'rgba(0,217,126,0.2)');
      html += card('Total Turns', p.total_turns.toLocaleString(), '$' + p.total_cost.toFixed(2) + ' total cost', 'var(--accent)', 'rgba(59,130,246,0.08)', 'rgba(59,130,246,0.2)');

      // Row 2: Efficiency + negative
      html += card('Avg Turns/Task', (p.avg_turns_per_task || 0).toFixed(1), '$' + (p.cost_per_completion || 0).toFixed(2) + ' per task', 'var(--yellow)', 'rgba(245,158,11,0.08)', 'rgba(245,158,11,0.2)');
      html += card('Failed', p.tasks_failed, p.rework_runs + ' rework runs', p.tasks_failed > 0 ? 'var(--red)' : 'var(--text-muted)', 'rgba(230,55,87,0.08)', 'rgba(230,55,87,0.2)');
      html += card('Amnesia', p.amnesia_count.toLocaleString(), 'rule violations', p.amnesia_count > 0 ? 'var(--red)' : 'var(--text-muted)', 'rgba(230,55,87,0.08)', 'rgba(230,55,87,0.2)');
      html += card('Watcher Failures', p.watcher_failures, 'audit failures', p.watcher_failures > 0 ? 'var(--red)' : 'var(--text-muted)', 'rgba(230,55,87,0.08)', 'rgba(230,55,87,0.2)');

      html += '</div>';
      breakdownEl.innerHTML = html;
    }

    // Daily table from trend data
    var tableEl = document.getElementById('prod-daily-table');
    if (tableEl && p.trend && p.trend.length > 0) {
      var thtml = '<div style="max-height:400px;overflow-y:auto"><table style="width:100%;border-collapse:collapse">' +
        '<thead style="position:sticky;top:0;background:var(--bg-primary);z-index:1"><tr>' +
          '<th class="th-left">date</th>' +
          '<th class="th-right">score</th>' +
          '<th class="th-right">completed</th>' +
          '<th class="th-right">failed</th>' +
          '<th class="th-right">success rate</th>' +
          '<th class="th-right">cost</th>' +
          '<th style="padding:6px 12px;text-align:left;width:120px">bar</th>' +
        '</tr></thead><tbody>';

      // Reverse: most recent first
      var trend = p.trend.slice().reverse();
      for (var i = 0; i < trend.length; i++) {
        var d = trend[i];
        var total = d.tasks_completed + d.tasks_failed;
        var rate = total > 0 ? Math.round(d.tasks_completed / total * 100) : 0;
        var rateColor = rate >= 80 ? 'var(--green)' : rate >= 60 ? 'var(--yellow)' : 'var(--red)';
        var barW = total > 0 ? Math.max(4, rate) : 0;

        thtml += '<tr style="border-bottom:1px solid var(--border)">' +
          '<td style="padding:8px 12px;font-family:var(--font-mono);font-size:12px">' + esc(d.date) + '</td>' +
          '<td style="padding:8px 12px;text-align:right;font-weight:600;color:' + rateColor + '">' + d.score + '</td>' +
          '<td style="padding:8px 12px;text-align:right;color:var(--green)">' + d.tasks_completed + '</td>' +
          '<td style="padding:8px 12px;text-align:right;color:' + (d.tasks_failed > 0 ? 'var(--red)' : 'var(--text-muted)') + '">' + d.tasks_failed + '</td>' +
          '<td style="padding:8px 12px;text-align:right;font-weight:500;color:' + rateColor + '">' + rate + '%</td>' +
          '<td style="padding:8px 12px;text-align:right;font-family:var(--font-mono);font-size:12px;color:var(--green)">$' + d.cost.toFixed(2) + '</td>' +
          '<td style="padding:8px 12px"><div style="height:12px;border-radius:2px;background:var(--bg-secondary);overflow:hidden"><div style="height:100%;width:' + barW + '%;background:' + rateColor + ';border-radius:2px"></div></div></td>' +
        '</tr>';
      }
      thtml += '</tbody></table></div>';
      tableEl.innerHTML = thtml;
    } else if (tableEl) {
      tableEl.innerHTML = '<div style="padding:16px;color:var(--text-muted);text-align:center">No productivity data yet. Run some tasks first.</div>';
    }
  };

  function card(label, value, sub, color, bg, border) {
    return '<div style="background:' + bg + ';border:1px solid ' + border + ';border-radius:6px;padding:12px">' +
      '<div style="font-size:11px;color:var(--text-muted);margin-bottom:4px">' + label + '</div>' +
      '<div style="font-size:22px;font-weight:600;color:' + color + '">' + value + '</div>' +
      '<div style="font-size:10px;color:var(--text-muted);margin-top:2px">' + sub + '</div>' +
    '</div>';
  }

  // Back button
  OL.onView(document.getElementById('prod-back-btn'), 'click', function() { OL.switchView('overview'); });

})(window.OL);
