(function () {
  'use strict';

  var cache = {}; // jid -> array of strings (phones)

  function fetchMembers(jid, cb) {
    if (cache[jid]) {
      return cb(null, cache[jid]);
    }
    fetch('/api/groups/' + encodeURIComponent(jid) + '/members')
      .then(function(res) {
        if (!res.ok) throw new Error('Network error');
        return res.json();
      })
      .then(function(data) {
        cache[jid] = data;
        cb(null, data);
      })
      .catch(function(err) {
        cb(err, null);
      });
  }

  function setupWidget(rootEl) {
    var jid = rootEl.getAttribute('data-group');
    var selectedRaw = rootEl.getAttribute('data-selected');
    var selectedPhones = selectedRaw ? selectedRaw.split(',').map(function(s) { return s.trim(); }).filter(Boolean) : [];
    
    var state = {
      open: false,
      members: null, // null means not loaded yet
      loading: false,
      error: false,
      selected: {}
    };
    for (var i=0; i<selectedPhones.length; i++) {
      state.selected[selectedPhones[i]] = true;
    }

    // Render basic skeleton
    var field = document.createElement('div');
    field.className = 'ap-field';
    var disp = document.createElement('div');
    disp.className = 'ap-display-text';
    var chev = document.createElement('div');
    chev.className = 'ap-chevron';
    chev.textContent = '▼';
    field.appendChild(disp);
    field.appendChild(chev);
    
    var panel = document.createElement('div');
    panel.className = 'ap-panel';
    
    // We will place hidden inputs inside the rootEl so form submission works
    var hiddenContainer = document.createElement('div');
    
    rootEl.appendChild(field);
    rootEl.appendChild(panel);
    rootEl.appendChild(hiddenContainer);
    
    function render() {
      // Update display text
      var selList = Object.keys(state.selected);
      if (selList.length === 0) {
        disp.textContent = 'Select assignees...';
        disp.style.color = 'var(--muted)';
      } else {
        disp.textContent = selList.join(', ');
        disp.style.color = 'var(--fg)';
      }

      // Update hidden inputs
      hiddenContainer.innerHTML = '';
      if (selList.length === 0) {
        // Need to submit empty if empty to clear the assignees (or fail validation)
        var inp = document.createElement('input');
        inp.type = 'hidden';
        inp.name = 'assign';
        inp.value = '';
        hiddenContainer.appendChild(inp);
      } else {
        selList.forEach(function(phone) {
          var inp = document.createElement('input');
          inp.type = 'hidden';
          inp.name = 'assign';
          inp.value = phone;
          hiddenContainer.appendChild(inp);
        });
      }

      // Update panel
      if (state.open) {
        panel.classList.add('open');
        panel.innerHTML = '';
        
        if (state.loading) {
          var loadEl = document.createElement('div');
          loadEl.className = 'ap-loading';
          loadEl.textContent = 'Loading members...';
          panel.appendChild(loadEl);
        } else if (state.error) {
          var errEl = document.createElement('div');
          errEl.className = 'ap-loading';
          errEl.style.color = 'var(--danger)';
          errEl.textContent = 'Failed to load members.';
          panel.appendChild(errEl);
        } else if (state.members) {
          if (state.members.length === 0) {
            var emptyEl = document.createElement('div');
            emptyEl.className = 'ap-loading';
            emptyEl.textContent = 'No members found.';
            panel.appendChild(emptyEl);
          } else {
            state.members.forEach(function(phone) {
              var btn = document.createElement('button');
              btn.type = 'button';
              btn.className = 'ap-opt';
              if (state.selected[phone]) {
                btn.classList.add('selected');
              }
              btn.textContent = phone;
              btn.addEventListener('click', function(e) {
                e.stopPropagation();
                if (state.selected[phone]) {
                  delete state.selected[phone];
                } else {
                  state.selected[phone] = true;
                }
                render();
              });
              panel.appendChild(btn);
            });
          }
        }
      } else {
        panel.classList.remove('open');
      }
    }

    field.addEventListener('click', function(e) {
      e.stopPropagation();
      state.open = !state.open;
      if (state.open && !state.members && !state.loading) {
        state.loading = true;
        render();
        fetchMembers(jid, function(err, data) {
          state.loading = false;
          if (err) {
            state.error = true;
          } else {
            state.members = data;
          }
          render();
        });
      } else {
        render();
      }
    });

    rootEl.__ap = {
      close: function() {
        state.open = false;
        render();
      }
    };

    render();
  }

  document.addEventListener('click', function(e) {
    var roots = document.querySelectorAll('.ap-wrap');
    for (var i=0; i<roots.length; i++) {
      if (!roots[i].contains(e.target) && roots[i].__ap) {
        roots[i].__ap.close();
      }
    }
  });

  document.addEventListener('DOMContentLoaded', function () {
    var roots = document.querySelectorAll('.ap-wrap');
    for (var i = 0; i < roots.length; i++) setupWidget(roots[i]);
  });

})();