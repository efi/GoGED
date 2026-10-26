// goged in the browser: draws the screens the server sends on a canvas,
// cell by cell like a terminal, and sends keys, the mouse and pasted text
// back. The page talks to its own server only.
'use strict';

(() => {
  const FG = '#dcdcdc';
  const BG = '#1e1e24';
  const SIZE = 15; // font size in CSS pixels
  const BOLD = 1, FAINT = 2, ITALIC = 4, UNDERLINE = 8, REVERSE = 16, STRIKE = 32;

  const canvas = document.getElementById('screen');
  const ctx = canvas.getContext('2d', { alpha: false });
  const note = document.getElementById('note');
  const login = document.getElementById('login');
  const tokenInput = document.getElementById('token');
  const loginError = document.getElementById('login-error');

  let cw = 8, ch = 20, dpr = 1, cols = 80, rows = 24;
  let lines = []; // the runs of every row, as last sent
  let ws = null;
  let token = '';
  let session = sessionStorage.getItem('goged-session') || '';
  let retries = 0;
  let ended = false;
  let download = null; // the file announced, whose bytes come next
  let noteAction = null;

  // --- The token ---------------------------------------------------------

  // The token comes in the address after #, which is never sent to the
  // server; the page keeps it for this tab and takes it out of the address.
  function takeToken() {
    if (location.hash.length > 1) {
      token = decodeURIComponent(location.hash.slice(1));
      sessionStorage.setItem('goged-token', token);
      history.replaceState(null, '', location.pathname + location.search);
    } else {
      token = sessionStorage.getItem('goged-token') || '';
    }
  }

  function showLogin(error) {
    loginError.textContent = error;
    login.hidden = false;
    note.hidden = true;
    tokenInput.focus();
  }

  login.addEventListener('submit', (e) => {
    e.preventDefault();
    token = tokenInput.value.trim();
    if (!token) return;
    sessionStorage.setItem('goged-token', token);
    tokenInput.value = '';
    login.hidden = true;
    connect();
  });

  function showNote(text, action) {
    note.textContent = text;
    note.hidden = false;
    noteAction = action || null;
  }

  note.addEventListener('click', () => {
    if (noteAction) {
      const action = noteAction;
      note.hidden = true;
      noteAction = null;
      action();
    }
  });

  // --- Drawing -----------------------------------------------------------

  function font(attrs) {
    return (attrs & ITALIC ? 'italic ' : '') + (attrs & BOLD ? 'bold ' : '') + SIZE + 'px "Martian Mono", monospace';
  }

  // px rounds a length to whole device pixels, for crisp lines.
  const px = (v) => Math.round(v * dpr) / dpr;

  // layout fits the grid of cells to the window.
  function layout() {
    dpr = window.devicePixelRatio || 1;
    ctx.font = font(0);
    cw = px(ctx.measureText('M').width);
    ch = px(Math.ceil(SIZE * 1.35));
    cols = Math.max(20, Math.floor(window.innerWidth / cw));
    rows = Math.max(5, Math.floor(window.innerHeight / ch));
    canvas.style.width = cols * cw + 'px';
    canvas.style.height = rows * ch + 'px';
    canvas.width = Math.round(cols * cw * dpr);
    canvas.height = Math.round(rows * ch * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.textBaseline = 'middle';
    widths.clear();
    for (let y = 0; y < rows; y++) drawRow(y);
  }

  const widths = new Map(); // glyph widths per font
  function glyphWidth(c, f) {
    const key = f + '\u0000' + c;
    let w = widths.get(key);
    if (w === undefined) {
      w = ctx.measureText(c).width;
      widths.set(key, w);
    }
    return w;
  }

  function drawRow(y) {
    const top = y * ch;
    ctx.globalAlpha = 1;
    ctx.fillStyle = BG;
    ctx.fillRect(0, top, cols * cw, ch);
    let x = 0;
    for (const [text, fg0, bg0, attrs, wide] of lines[y] || []) {
      let fg = fg0 || FG, bg = bg0;
      if (attrs & REVERSE) [fg, bg] = [bg || BG, fg];
      const cells = wide ? [text] : Array.from(text);
      const n = wide || cells.length;
      if (bg) {
        ctx.fillStyle = bg;
        ctx.fillRect(x * cw, top, n * cw, ch);
      }
      ctx.globalAlpha = attrs & FAINT ? 0.6 : 1;
      ctx.fillStyle = fg;
      ctx.strokeStyle = fg;
      const f = font(attrs);
      ctx.font = f;
      let cx = x;
      for (const c of cells) {
        drawCell(c, cx * cw, top, (wide || 1) * cw, f, attrs);
        cx += wide || 1;
      }
      if (attrs & UNDERLINE) ctx.fillRect(x * cw, top + ch - px(2), n * cw, px(1));
      if (attrs & STRIKE) ctx.fillRect(x * cw, top + px(ch / 2), n * cw, px(1));
      x += n;
    }
    ctx.globalAlpha = 1;
  }

  function drawCell(c, x, y, w, f, attrs) {
    if (c === ' ') return;
    const cp = c.codePointAt(0);
    if (cp >= 0x2800 && cp <= 0x28ff && c.length === 1) return braille(cp - 0x2800, x, y);
    if (cp >= 0x2500 && cp <= 0x257f && c.length === 1 && boxDrawing(cp, x, y, attrs)) return;
    const gw = glyphWidth(c, f);
    if (gw > w + 0.5) {
      // A glyph from another font, wider than its cell: squeezed into it.
      ctx.save();
      ctx.translate(x, y + ch / 2);
      ctx.scale(w / gw, 1);
      ctx.fillText(c, 0, 0);
      ctx.restore();
    } else {
      ctx.fillText(c, x + (w - gw) / 2, y + ch / 2);
    }
  }

  // braille draws the dots of a Braille pattern, two columns of four, so
  // that the map needs no font and its dots line up across cells.
  const DOTS = [[0, 0], [0, 1], [0, 2], [1, 0], [1, 1], [1, 2], [0, 3], [1, 3]];
  function braille(bits, x, y) {
    const s = Math.max(1, px(Math.min(cw / 2, ch / 4) * 0.55));
    for (let i = 0; i < 8; i++) {
      if (bits & (1 << i)) {
        const [col, row] = DOTS[i];
        ctx.fillRect(px(x + cw * (0.25 + col / 2) - s / 2), px(y + ch * (0.125 + row / 4) - s / 2), s, s);
      }
    }
  }

  // Box drawing: the arms of each character (up, right, down, left; 1
  // light, 2 heavy), dashes, rounded corners. Drawn, the lines meet across
  // cells whatever the line height.
  const BOX = {
    0x2500: '0101', 0x2501: '0202', 0x2502: '1010', 0x2503: '2020',
    0x250c: '0110', 0x2510: '0011', 0x2514: '1100', 0x2518: '1001',
    0x250f: '0220', 0x2513: '0022', 0x2517: '2200', 0x251b: '2002',
    0x251c: '1110', 0x2524: '1011', 0x252c: '0111', 0x2534: '1101', 0x253c: '1111',
    0x2523: '2220', 0x252b: '2022', 0x2533: '0222', 0x253b: '2202', 0x254b: '2222',
    0x2574: '0001', 0x2575: '1000', 0x2576: '0100', 0x2577: '0010',
    0x2578: '0002', 0x2579: '2000', 0x257a: '0200', 0x257b: '0020',
    0x256d: '0110', 0x256e: '0011', 0x256f: '1001', 0x2570: '1100',
  };
  const DASHES = {
    0x2504: 3, 0x2505: 3, 0x2506: 3, 0x2507: 3, 0x2508: 4, 0x2509: 4, 0x250a: 4, 0x250b: 4,
    0x254c: 2, 0x254d: 2, 0x254e: 2, 0x254f: 2,
  };
  const DASHED = {
    0x2504: '0101', 0x2505: '0202', 0x2506: '1010', 0x2507: '2020', 0x2508: '0101', 0x2509: '0202',
    0x250a: '1010', 0x250b: '2020', 0x254c: '0101', 0x254d: '0202', 0x254e: '1010', 0x254f: '2020',
  };

  function boxDrawing(cp, x, y) {
    const arms = BOX[cp] || DASHED[cp];
    if (!arms) return false;
    const light = Math.max(1, px(ch / 18));
    const heavy = Math.max(2, px(ch / 7));
    const cx = px(x + cw / 2), cy = px(y + ch / 2);
    const [up, right, down, left] = Array.from(arms, Number);
    const weight = (a) => (a === 2 ? heavy : light);
    if (cp >= 0x256d && cp <= 0x2570) {
      // A rounded corner: from the middle of one side to the middle of the
      // other, round in the middle.
      const r = Math.min(cw, ch) / 2;
      ctx.lineWidth = light;
      ctx.beginPath();
      ctx.moveTo(right ? x + cw : x, cy);
      ctx.arcTo(cx, cy, cx, up ? y : y + ch, r);
      ctx.lineTo(cx, up ? y : y + ch);
      ctx.stroke();
      return true;
    }
    const dashes = DASHES[cp];
    if (dashes) {
      const horizontal = right > 0;
      const w = weight(horizontal ? right : up);
      const len = (horizontal ? cw : ch) / dashes;
      for (let i = 0; i < dashes; i++) {
        if (horizontal) ctx.fillRect(px(x + i * len + len * 0.2), cy - w / 2, px(len * 0.6), w);
        else ctx.fillRect(cx - w / 2, px(y + i * len + len * 0.2), w, px(len * 0.6));
      }
      return true;
    }
    // The arms overlap in the middle by half the widest line, so that
    // corners close.
    const mid = Math.max(up && weight(up), down && weight(down), left && weight(left), right && weight(right)) / 2;
    if (up) ctx.fillRect(cx - weight(up) / 2, y, weight(up), cy - y + mid);
    if (down) ctx.fillRect(cx - weight(down) / 2, cy - mid, weight(down), y + ch - cy + mid);
    if (left) ctx.fillRect(x, cy - weight(left) / 2, cx - x + mid, weight(left));
    if (right) ctx.fillRect(cx - mid, cy - weight(right) / 2, x + cw - cx + mid, weight(right));
    return true;
  }

  // --- The connection ----------------------------------------------------

  function send(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
  }

  function connect() {
    ended = false;
    const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(scheme + '//' + location.host + '/ws');
    ws.binaryType = 'arraybuffer';
    // The page speaks first: the token, and the session to carry on.
    ws.onopen = () => ws.send(JSON.stringify({ token, session, cols, rows }));
    ws.onmessage = (e) => {
      if (e.data instanceof ArrayBuffer) {
        if (download) save(download, e.data);
        download = null;
        return;
      }
      const m = JSON.parse(e.data);
      switch (m.t) {
        case 'hello':
          session = m.session;
          sessionStorage.setItem('goged-session', session);
          document.title = m.title;
          retries = 0;
          note.hidden = true;
          canvas.focus();
          break;
        case 'title':
          document.title = m.title;
          break;
        case 'frame':
          lines.length = m.rows;
          for (const [i, runs] of m.lines) {
            lines[i] = runs;
            if (i < rows) drawRow(i);
          }
          break;
        case 'download':
          download = m;
          break;
        case 'bye':
          end();
          break;
      }
    };
    ws.onclose = (e) => {
      ws = null;
      if (ended) return;
      switch (e.code) {
        case 4001:
          token = '';
          sessionStorage.removeItem('goged-token');
          showLogin('The token is not right.');
          return;
        case 4002:
          showNote('goged is open in another tab. Click here to use it in this one.', connect);
          return;
        case 4003:
          showNote('The server has too many sessions open. Click here to try again.', connect);
          return;
        case 4004:
          end();
          return;
      }
      showNote('The connection to the server is lost; trying again …');
      setTimeout(connect, Math.min(10000, 500 * 2 ** retries++));
    };
  }

  function end() {
    ended = true;
    session = '';
    sessionStorage.removeItem('goged-session');
    showNote('goged has ended. Click here to start again.', () => connect());
  }

  // save hands a file the user exported or printed to the browser.
  function save(d, data) {
    const url = URL.createObjectURL(new Blob([data], { type: d.type }));
    const a = document.createElement('a');
    a.href = url;
    a.download = d.name;
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30000);
  }

  // --- Keys ----------------------------------------------------------------

  const NAMED = {
    ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right',
    Enter: 'enter', Escape: 'esc', Backspace: 'backspace', Delete: 'delete', Insert: 'insert',
    Tab: 'tab', Home: 'home', End: 'end', PageUp: 'pgup', PageDown: 'pgdown',
    F1: 'f1', F2: 'f2', F3: 'f3', F4: 'f4', F6: 'f6', F7: 'f7', F8: 'f8', F9: 'f9', F10: 'f10',
  };
  const MOVES = new Set(['up', 'down', 'left', 'right', 'home', 'end', 'pgup', 'pgdown']);
  // ctrl with these letters stays with the browser: copying and pasting
  // (pastes come as text), and the keys browsers keep for themselves.
  const BROWSER = 'cvxrwtn';

  // keyName names a key as Bubble Tea does, or returns null for keys the
  // browser keeps.
  function keyName(e) {
    if (e.isComposing || e.metaKey || e.key === 'Dead' || e.key === 'Process') return null;
    const altGraph = e.getModifierState && e.getModifierState('AltGraph');
    const alt = e.altKey && !altGraph, ctrl = e.ctrlKey && !altGraph, shift = e.shiftKey;
    let name = NAMED[e.key];
    if (name) {
      if (name === 'tab' && shift) name = 'shift+tab';
      else if (MOVES.has(name) && (ctrl || shift)) name = (ctrl ? 'ctrl+' : '') + (shift ? 'shift+' : '') + name;
      return (alt ? 'alt+' : '') + name;
    }
    let c = e.key;
    if (Array.from(c).length !== 1) return null;
    if (ctrl) {
      const l = c.toLowerCase();
      if (l < 'a' || l > 'z' || BROWSER.includes(l)) return null;
      return (alt ? 'alt+' : '') + 'ctrl+' + l;
    }
    if (alt) {
      // Option on a Mac types other characters: the key's place tells.
      const m = /^Key([A-Z])$/.exec(e.code) || /^Digit([0-9])$/.exec(e.code);
      if (m) c = shift ? m[1] : m[1].toLowerCase();
      return 'alt+' + c;
    }
    return c;
  }

  window.addEventListener('keydown', (e) => {
    if (!login.hidden) return;
    const name = keyName(e);
    if (name && ws) {
      e.preventDefault();
      send({ t: 'key', key: name });
    }
  });

  window.addEventListener('paste', (e) => {
    if (!login.hidden) return;
    const text = e.clipboardData && e.clipboardData.getData('text');
    if (text && ws) {
      e.preventDefault();
      send({ t: 'paste', text });
    }
  });

  // --- The mouse -----------------------------------------------------------

  const BUTTONS = ['left', 'middle', 'right', 'backward', 'forward'];
  let lastCell = '';

  function mouse(e, button, action) {
    const r = canvas.getBoundingClientRect();
    const x = Math.floor((e.clientX - r.left) / cw), y = Math.floor((e.clientY - r.top) / ch);
    if (action === 'motion') {
      if (x + ',' + y === lastCell) return;
    }
    lastCell = x + ',' + y;
    send({ t: 'mouse', x, y, button, action, alt: e.altKey, ctrl: e.ctrlKey, shift: e.shiftKey });
  }

  canvas.addEventListener('mousedown', (e) => {
    e.preventDefault();
    canvas.focus();
    if (BUTTONS[e.button]) mouse(e, BUTTONS[e.button], 'press');
  });
  window.addEventListener('mouseup', (e) => {
    if (e.button === 3 || e.button === 4) e.preventDefault(); // not back and forward in the browser
    if (BUTTONS[e.button] && login.hidden) mouse(e, BUTTONS[e.button], 'release');
  });
  canvas.addEventListener('mousemove', (e) => {
    if (!e.buttons) return;
    const b = e.buttons & 1 ? 'left' : e.buttons & 4 ? 'middle' : e.buttons & 2 ? 'right' : 'none';
    mouse(e, b, 'motion');
  });
  canvas.addEventListener('contextmenu', (e) => e.preventDefault());

  // The wheel: a step for every notch, or for so much of a smooth scroll.
  let wheelX = 0, wheelY = 0;
  canvas.addEventListener('wheel', (e) => {
    e.preventDefault();
    const scale = e.deltaMode === 1 ? 40 : e.deltaMode === 2 ? 400 : 1;
    wheelY += e.deltaY * scale;
    wheelX += e.deltaX * scale;
    const step = 40;
    for (; Math.abs(wheelY) >= step; wheelY -= Math.sign(wheelY) * step) mouse(e, wheelY < 0 ? 'wheelup' : 'wheeldown', 'press');
    for (; Math.abs(wheelX) >= step; wheelX -= Math.sign(wheelX) * step) mouse(e, wheelX < 0 ? 'wheelleft' : 'wheelright', 'press');
  }, { passive: false });

  // --- Start -------------------------------------------------------------

  let resizing = 0;
  window.addEventListener('resize', () => {
    clearTimeout(resizing);
    resizing = setTimeout(() => {
      layout();
      send({ t: 'size', cols, rows });
    }, 50);
  });

  takeToken();
  Promise.all([document.fonts.load(font(0)), document.fonts.load(font(BOLD))])
    .catch(() => {})
    .then(() => {
      layout();
      if (token) connect();
      else showLogin('');
    });
})();
