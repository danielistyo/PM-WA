/*
 * PM-WA reminder picker.
 *
 * A react-js-cron-style editor rendered in vanilla JS, paired with the
 * vendored cronstrue library (web/static/cronstrue.min.js) so the field
 * always shows a humanized value while the submitted value stays a 5-field
 * cron expression (the format stored in the DB).
 *
 * Structure:
 *   - A field button showing the humanized value; clicking it toggles a
 *     dropdown panel.
 *   - The panel holds quick options (No reminder / At deadline / Once at) and
 *     the cron builder: "Every [period]" with contextual MULTI-select fields
 *     (month, month-day, weekday, hour, minute) that appear based on the
 *     selected period, plus a "Cron expression" free-text toggle.
 *
 * Submitted values (single hidden "reminder" input):
 *   No reminder -> "no"
 *   At deadline -> "yes"
 *   Once at     -> "2006-01-02 15:04" (absolute, GMT+7)
 *   Recurring   -> 5-field cron expression (e.g. "0 9,18 * * 1,3,5")
 */
(function (root, factory) {
  if (typeof module === 'object' && module.exports) {
    // Node.js (used by tests)
    module.exports = factory();
  } else {
    // Browser
    var api = factory();
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', function () {
        api.initAll();
      });
    } else {
      api.initAll();
    }
  }
})(typeof self !== 'undefined' ? self : this, function () {
  'use strict';

  var MONTHS = ['January', 'February', 'March', 'April', 'May', 'June', 'July',
    'August', 'September', 'October', 'November', 'December'];
  var MONTHS_SHORT = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
    'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  var WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
  var WEEKDAYS_SHORT = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  var WEEKDAY_VALUES = ['0', '1', '2', '3', '4', '5', '6']; // 0 = Sunday

  // Period options in react-js-cron order.
  var PERIODS = [
    { value: 'minute', label: 'minute' },
    { value: 'hour', label: 'hour' },
    { value: 'day', label: 'day' },
    { value: 'week', label: 'week' },
    { value: 'month', label: 'month' },
    { value: 'year', label: 'year' }
  ];

  // Which multi-select fields are visible for each period, mirroring react-js-cron.
  var PERIOD_SELECTS = {
    minute: [],
    hour: ['minute'],
    day: ['hour', 'minute'],
    week: ['weekday', 'hour', 'minute'],
    month: ['monthday', 'hour', 'minute'],
    year: ['month', 'monthday', 'hour', 'minute']
  };

  // Default single value used when a newly visible field has no selection.
  var DEFAULTS = { minute: '0', hour: '0', monthday: '1', month: '1', weekday: '1' };

  var CRON_KEYS = ['minute', 'hour', 'dom', 'month', 'dow'];
  var CRON_RANGES = { minute: [0, 59], hour: [0, 23], dom: [1, 31], month: [1, 12], dow: [0, 6] };

  function pad(n) {
    return (n < 10 ? '0' : '') + n;
  }

  function partValid(part, range) {
    var m;
    if (part === '*') return true;
    // step-from-start: */15
    if ((m = part.match(/^\*\/(\d+)$/))) {
      return parseInt(m[1], 10) >= 1;
    }
    var tokens = String(part).split(',');
    for (var i = 0; i < tokens.length; i++) {
      var t = tokens[i];
      if (t === '') return false;
      m = t.match(/^(\d+)(?:-(\d+))?(?:\/(\d+))?$/);
      if (!m) return false;
      var lo = parseInt(m[1], 10);
      if (lo < range[0] || lo > range[1]) return false;
      if (m[2] !== undefined) {
        var hi = parseInt(m[2], 10);
        if (hi < range[0] || hi > range[1] || hi < lo) return false;
      }
      if (m[3] !== undefined) {
        var step = parseInt(m[3], 10);
        if (step < 1) return false;
      }
    }
    return true;
  }

  // Accepts only standard 5-field cron expressions (matches backend parse).
  function isValidCron5(input) {
    var p = String(input || '').trim().split(/\s+/);
    if (p.length !== 5) return false;
    for (var i = 0; i < 5; i++) {
      if (!partValid(p[i], CRON_RANGES[CRON_KEYS[i]])) return false;
    }
    return true;
  }

  function humanize(cron) {
    if (typeof cronstrue === 'undefined' || !cronstrue || !cronstrue.toString) return null;
    try {
      return cronstrue.toString(cron, { use24HourTimeFormat: true });
    } catch (err) {
      return null;
    }
  }

  function parseCronParts(str) {
    var p = String(str || '').trim().split(/\s+/);
    if (p.length !== 5) return null;
    return { minute: p[0], hour: p[1], dom: p[2], month: p[3], dow: p[4] };
  }

  function periodFromParts(parts) {
    var isEvery = function (v) { return v === '*'; };
    if (isEvery(parts.minute) && isEvery(parts.hour) && isEvery(parts.dom) &&
        isEvery(parts.month) && isEvery(parts.dow)) return 'minute';
    if (isEvery(parts.hour) && isEvery(parts.dom) && isEvery(parts.month) &&
        isEvery(parts.dow)) return 'hour';
    if (isEvery(parts.dom) && isEvery(parts.month) && isEvery(parts.dow)) return 'day';
    if (isEvery(parts.dom) && isEvery(parts.month)) return 'week';
    if (isEvery(parts.month)) return 'month';
    return 'year';
  }

  // A selected-values array -> sorted, deduped comma list, or "*" when empty.
  function fieldToCron(arr) {
    if (!arr || !arr.length) return '*';
    var nums = arr.map(function (x) { return parseInt(x, 10); })
      .sort(function (a, b) { return a - b; });
    var out = [];
    for (var i = 0; i < nums.length; i++) {
      if (nums[i] !== out[out.length - 1]) out.push(nums[i]);
    }
    return out.join(',');
  }

  function buildCronFromState(period, values) {
    var v = values || {};
    var minute = fieldToCron(v.minute);
    var hour = fieldToCron(v.hour);
    var dom = fieldToCron(v.monthday);
    var month = fieldToCron(v.month);
    var dow = fieldToCron(v.weekday);
    switch (period) {
      case 'minute': return '* * * * *';
      case 'hour': return minute + ' * * * *';
      case 'day': return minute + ' ' + hour + ' * * *';
      case 'week': return minute + ' ' + hour + ' * * ' + dow;
      case 'month': return minute + ' ' + hour + ' ' + dom + ' * *';
      case 'year': return minute + ' ' + hour + ' ' + dom + ' ' + month + ' *';
    }
    return '* * * * *';
  }

  // Comma-separated numeric list -> sorted string array ([] for "*"), or null.
  function parseFieldList(part, range) {
    if (part === '*') return [];
    var tokens = String(part).split(',');
    var nums = [];
    for (var i = 0; i < tokens.length; i++) {
      if (!/^\d+$/.test(tokens[i])) return null;
      var n = parseInt(tokens[i], 10);
      if (n < range[0] || n > range[1]) return null;
      if (nums.indexOf(n) === -1) nums.push(n);
    }
    nums.sort(function (a, b) { return a - b; });
    return nums;
  }

  function toStrArr(nums) {
    return nums.map(String);
  }

  // Map a cron expression onto picker state when it can be represented by the
  // multi-selects (each field either "*" or a comma list of plain integers).
  // Otherwise returns null and the caller falls back to the free-text input.
  function stateFromCron(cron) {
    var parts = parseCronParts(cron);
    if (!parts) return null;
    var minute = parseFieldList(parts.minute, CRON_RANGES.minute);
    var hour = parseFieldList(parts.hour, CRON_RANGES.hour);
    var dom = parseFieldList(parts.dom, CRON_RANGES.dom);
    var month = parseFieldList(parts.month, CRON_RANGES.month);
    var dow = parseFieldList(parts.dow, CRON_RANGES.dow);
    if (!minute || !hour || !dom || !month || !dow) return null;
    return {
      period: periodFromParts(parts),
      values: {
        minute: toStrArr(minute),
        hour: toStrArr(hour),
        monthday: toStrArr(dom),
        month: toStrArr(month),
        weekday: toStrArr(dow)
      }
    };
  }

  function addOption(sel, value, label) {
    var opt = document.createElement('option');
    opt.value = value;
    opt.textContent = label;
    sel.appendChild(opt);
  }

  // Options for each multi-select field, with short labels for the closed state.
  var MULTI_OPTIONS = {
    month: MONTHS.map(function (m, i) {
      return { value: String(i + 1), label: m, short: MONTHS_SHORT[i] };
    }),
    monthday: (function () {
      var out = [];
      for (var d = 1; d <= 31; d++) {
        out.push({ value: String(d), label: String(d), short: String(d) });
      }
      return out;
    })(),
    weekday: WEEKDAY_VALUES.map(function (v, i) {
      return { value: v, label: WEEKDAYS[i], short: WEEKDAYS_SHORT[i] };
    }),
    hour: (function () {
      var out = [];
      for (var h = 0; h < 24; h++) out.push({ value: String(h), label: pad(h), short: pad(h) });
      return out;
    })(),
    minute: (function () {
      var out = [];
      for (var m = 0; m < 60; m++) out.push({ value: String(m), label: pad(m), short: pad(m) });
      return out;
    })()
  };

  // Close every widget's panels when clicking outside of it.
  var globalClickBound = false;
  function bindGlobalClick() {
    if (globalClickBound || typeof document === 'undefined') return;
    globalClickBound = true;
    document.addEventListener('click', function (e) {
      var roots = document.querySelectorAll('[data-reminder-field]');
      for (var i = 0; i < roots.length; i++) {
        var r = roots[i];
        if (!r.contains(e.target) && r.__rp) r.__rp.close();
      }
    });
  }

  function setupWidget(rootEl) {
    if (rootEl.__rpInit) return;
    rootEl.__rpInit = true;

    var toggle = rootEl.querySelector('[data-rj-toggle]');
    var panel = rootEl.querySelector('[data-rj-panel]');
    var display = rootEl.querySelector('[data-rj-display]');
    var hidden = rootEl.querySelector('[data-reminder-value]');
    var quickBtns = Array.prototype.slice.call(
      rootEl.querySelectorAll('[data-rj-quick] [data-mode]'));
    var atWrap = rootEl.querySelector('[data-reminder-at]');
    var atInput = rootEl.querySelector('[data-reminder-at-input]');
    var cronWrap = rootEl.querySelector('[data-reminder-cron]');
    var row = rootEl.querySelector('[data-cron-row]');
    var periodSel = rootEl.querySelector('[data-cron-period]');
    var customBtn = rootEl.querySelector('[data-cron-custom-btn]');
    var customWrap = rootEl.querySelector('[data-cron-custom]');
    var customInput = rootEl.querySelector('[data-cron-custom-input]');
    var preview = rootEl.querySelector('[data-cron-preview]');
    var raw = rootEl.querySelector('[data-cron-raw]');
    var human = rootEl.querySelector('[data-cron-human]');
    var minutePrefix = rootEl.querySelector('[data-cron-minute-prefix]');
    var minuteSuffix = rootEl.querySelector('[data-cron-minute-suffix]');
    var fieldWraps = {};
    Array.prototype.slice.call(rootEl.querySelectorAll('[data-cron-field]')).forEach(function (el) {
      fieldWraps[el.getAttribute('data-cron-field')] = el;
    });

    var state = {
      open: false,
      mode: 'yes',
      atValue: '',
      period: 'day',
      cron: '0 0 * * *',
      values: { minute: ['0'], hour: ['0'], monthday: ['1'], month: ['1'], weekday: ['1'] },
      custom: false,
      customValue: '',
      cronError: false
    };

    var multis = {};
    function closeMultis(exceptKey) {
      Object.keys(multis).forEach(function (k) {
        if (k !== exceptKey) multis[k].close();
      });
    }

    // Build a react-js-cron-style multi-select for one cron field.
    function makeMulti(key, options) {
      var host = rootEl.querySelector('[data-multi="' + key + '"]');
      if (!host) return { close: function () {}, render: function () {} };
      host.innerHTML = '';

      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'rj-multi-btn';
      btn.setAttribute('aria-haspopup', 'listbox');

      var lbl = document.createElement('span');
      lbl.className = 'rj-multi-label';
      var clear = document.createElement('span');
      clear.className = 'rj-multi-clear';
      clear.textContent = '\u00d7';
      clear.setAttribute('role', 'button');
      clear.setAttribute('aria-label', 'Clear selection');
      var chev = document.createElement('span');
      chev.className = 'rj-chevron';
      chev.textContent = '\u25be';

      var panelEl = document.createElement('div');
      panelEl.className = 'rj-multi-panel';
      panelEl.hidden = true;

      var opts = options.map(function (o) {
        var ob = document.createElement('button');
        ob.type = 'button';
        ob.className = 'rj-multi-opt';
        ob.setAttribute('data-value', o.value);
        ob.textContent = o.label;
        panelEl.appendChild(ob);
        return { value: o.value, short: o.short, btn: ob };
      });

      btn.appendChild(lbl);
      btn.appendChild(clear);
      btn.appendChild(chev);
      host.appendChild(btn);
      host.appendChild(panelEl);

      function isOpen() { return !panelEl.hidden; }
      function setOpen(v) {
        if (v) closeMultis(key);
        panelEl.hidden = !v;
        btn.classList.toggle('open', !!v);
      }

      btn.addEventListener('click', function (e) {
        if (e.target === clear) return; // handled by the clear handler
        setOpen(!isOpen());
      });
      clear.addEventListener('click', function (e) {
        e.stopPropagation();
        state.values[key] = [];
        updateCron();
        renderAll();
      });
      opts.forEach(function (o) {
        o.btn.addEventListener('click', function () {
          var sel = state.values[key].slice();
          var idx = sel.indexOf(o.value);
          if (idx === -1) sel.push(o.value); else sel.splice(idx, 1);
          state.values[key] = sel;
          updateCron();
          renderAll(); // keep the options panel open
        });
      });

      function render() {
        var sel = state.values[key] || [];
        opts.forEach(function (o) {
          o.btn.classList.toggle('selected', sel.indexOf(o.value) !== -1);
        });
        if (!sel.length) {
          lbl.textContent = '*';
          lbl.classList.add('any');
        } else {
          var shorts = sel.map(function (v) {
            for (var i = 0; i < opts.length; i++) {
              if (opts[i].value === v) return opts[i].short;
            }
            return v;
          });
          lbl.textContent = shorts.join(', ');
          lbl.classList.remove('any');
        }
        btn.classList.toggle('has-values', sel.length > 0);
      }

      return {
        key: key,
        close: function () { setOpen(false); },
        render: render
      };
    }

    Object.keys(MULTI_OPTIONS).forEach(function (key) {
      multis[key] = makeMulti(key, MULTI_OPTIONS[key]);
    });

    function updateCron() {
      state.cron = buildCronFromState(state.period, state.values);
    }

    function renderCron() {
      periodSel.value = state.period;
      var visible = PERIOD_SELECTS[state.period] || [];
      Object.keys(fieldWraps).forEach(function (key) {
        fieldWraps[key].hidden = visible.indexOf(key) === -1;
      });
      minutePrefix.textContent = state.period === 'hour' ? 'at' : ':';
      minuteSuffix.hidden = state.period !== 'hour';
      customBtn.classList.toggle('active', state.custom);
      customBtn.textContent = state.custom ? 'Selector' : 'Cron expression';
      customWrap.hidden = !state.custom;
      row.hidden = state.custom;
      Object.keys(multis).forEach(function (k) { multis[k].render(); });

      if (state.custom) {
        if (document.activeElement !== customInput) {
          customInput.value = state.customValue;
        }
        if (state.cronError) {
          preview.classList.add('error');
          raw.textContent = state.customValue;
          human.textContent = 'Invalid cron expression';
        } else if (state.customValue) {
          preview.classList.remove('error');
          raw.textContent = state.customValue;
          human.textContent = humanize(state.customValue) || '';
        } else {
          preview.classList.remove('error');
          raw.textContent = '';
          human.textContent = '';
        }
        return;
      }

      preview.classList.remove('error');
      raw.textContent = state.cron;
      human.textContent = humanize(state.cron) || '';
    }

    function renderDisplay() {
      var text;
      var invalid = false;
      if (state.mode === 'no') {
        text = 'No reminder';
      } else if (state.mode === 'yes') {
        text = 'At task deadline';
      } else if (state.mode === 'at') {
        text = state.atValue ? 'Once at ' + state.atValue : 'Once at\u2026';
      } else {
        if (state.cronError) {
          text = 'Invalid cron expression';
          invalid = true;
        } else {
          text = humanize(state.cron) || state.cron;
        }
      }
      display.textContent = text;
      toggle.classList.toggle('error', invalid);
    }

    function syncHidden() {
      if (state.mode === 'no') hidden.value = 'no';
      else if (state.mode === 'yes') hidden.value = 'yes';
      else if (state.mode === 'at') hidden.value = state.atValue;
      else hidden.value = state.cron;
    }

    function renderAll() {
      quickBtns.forEach(function (b) {
        b.classList.toggle('active', b.getAttribute('data-mode') === state.mode);
      });
      panel.hidden = !state.open;
      toggle.setAttribute('aria-expanded', state.open ? 'true' : 'false');
      atWrap.hidden = state.mode !== 'at';
      cronWrap.hidden = state.mode !== 'cron';
      renderCron();
      renderDisplay();
      syncHidden();
    }

    // ---- event handlers ----
    toggle.addEventListener('click', function () {
      state.open = !state.open;
      if (!state.open) closeMultis(null);
      renderAll();
    });

    rootEl.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') rootEl.__rp.close();
    });

    quickBtns.forEach(function (b) {
      b.addEventListener('click', function () {
        state.mode = b.getAttribute('data-mode');
        renderAll();
      });
    });

    periodSel.addEventListener('change', function () {
      state.period = periodSel.value;
      // Newly visible fields start with a sensible default selection.
      PERIOD_SELECTS[state.period].forEach(function (key) {
        if (!state.values[key] || !state.values[key].length) {
          state.values[key] = [DEFAULTS[key]];
        }
      });
      updateCron();
      renderAll();
    });

    customBtn.addEventListener('click', function () {
      if (!state.custom) {
        // Enter free-text cron mode with the current select-driven cron.
        state.custom = true;
        state.customValue = state.cron;
        state.cronError = false;
      } else {
        // Try to leave free-text mode.
        if (isValidCron5(state.customValue)) {
          var st = stateFromCron(state.customValue);
          if (st) {
            // The expression maps cleanly back onto the multi-selects.
            state.custom = false;
            state.period = st.period;
            state.values = st.values;
            state.cron = state.customValue;
          } else {
            // Valid expression the multi-selects cannot express (e.g. */15):
            // discard and revert to the selector-driven cron.
            state.custom = false;
            state.cronError = false;
            updateCron();
          }
        } else {
          // Invalid text is discarded and the picker reverts to what the
          // multi-selects currently describe, keeping the value in sync.
          state.custom = false;
          state.cronError = false;
          updateCron();
        }
      }
      renderAll();
    });

    customInput.addEventListener('input', function () {
      state.customValue = customInput.value.trim();
      if (state.customValue === '') {
        state.cronError = false;
      } else if (isValidCron5(state.customValue)) {
        state.cron = state.customValue;
        state.cronError = false;
      } else {
        state.cronError = true;
      }
      renderAll();
    });

    atInput.addEventListener('change', function () {
      state.atValue = atInput.value ? atInput.value.replace('T', ' ') : '';
      renderAll();
    });

    // ---- initialise from the server-rendered value ----
    rootEl.__rp = {
      close: function () {
        state.open = false;
        closeMultis(null);
        renderAll();
      }
    };
    bindGlobalClick();

    PERIODS.forEach(function (p) { addOption(periodSel, p.value, p.label); });

    var initial = (rootEl.getAttribute('data-initial') || '').trim();
    var st;
    if (initial === '' || /^yes$/i.test(initial)) {
      state.mode = 'yes';
    } else if (/^no$/i.test(initial)) {
      state.mode = 'no';
    } else if (/^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}$/.test(initial)) {
      state.mode = 'at';
      state.atValue = initial.replace('T', ' ');
      atInput.value = initial.replace(' ', 'T');
    } else if ((st = stateFromCron(initial))) {
      state.mode = 'cron';
      state.period = st.period;
      state.values = st.values;
      state.cron = initial;
    } else if (isValidCron5(initial)) {
      state.mode = 'cron';
      state.custom = true;
      state.customValue = initial;
      state.cron = initial;
    } else {
      state.mode = 'yes';
    }
    renderAll();
  }

  function initAll() {
    if (typeof document === 'undefined') return;
    var roots = document.querySelectorAll('[data-reminder-field]');
    for (var i = 0; i < roots.length; i++) setupWidget(roots[i]);
  }

  return {
    initAll: initAll,
    humanize: humanize,
    isValidCron5: isValidCron5,
    parseCronParts: parseCronParts,
    periodFromParts: periodFromParts,
    fieldToCron: fieldToCron,
    buildCronFromState: buildCronFromState,
    stateFromCron: stateFromCron
  };
});





