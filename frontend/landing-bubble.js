(function () {
  const palette = {
    high: ['#0F1B6E', '#1A2878'],
    mid:  ['#5A54FF', '#6F6BFF', '#1FD3CC'],
    low:  ['#7A80D4', '#9DA3CC', '#6ECFCB', '#6170B8'],
  };

  // Headwords from a real collection with imaginary view counts —
  // simulating what a bubble looks like after a few weeks of practice.
  const words = [
    { label: 'anecdote',                    count: 22 },
    { label: 'ethos',                       count: 18 },
    { label: 'serendipitous',               count: 15 },
    { label: 'in the groove',               count: 13 },
    { label: 'fret',                        count: 12 },
    { label: 'presumptuous',                count: 11 },
    { label: 'oblivious',                   count: 10 },
    { label: 'epiphany',                    count:  9 },
    { label: 'egregious',                   count:  9 },
    { label: 'asinine',                     count:  8 },
    { label: 'squander',                    count:  8 },
    { label: 'disparage',                   count:  7 },
    { label: 'digression',                  count:  7 },
    { label: 'conflate',                    count:  7 },
    { label: 'behest',                      count:  6 },
    { label: 'proclivity',                  count:  6 },
    { label: 'pervasive',                   count:  6 },
    { label: 'unfettered vs inalienable',   count:  5 },
    { label: 'disheartening vs heartening', count:  5 },
    { label: 'equanimity',                  count:  5 },
    { label: 'debacle',                     count:  5 },
    { label: 'hubris',                      count:  5 },
    { label: 'tenuous',                     count:  5 },
    { label: 'dicey',                       count:  5 },
    { label: 'fortuitous vs serendipitous', count:  4 },
    { label: 'scrappy',                     count:  4 },
    { label: 'transpired',                  count:  4 },
    { label: 'conducive',                   count:  4 },
    { label: 'conspicuous',                 count:  4 },
    { label: 'hiatus',                      count:  4 },
    { label: 'beyond the pale',             count:  4 },
    { label: 'bona fide',                   count:  4 },
    { label: 'impunity',                    count:  3 },
    { label: 'meandering',                  count:  3 },
    { label: 'pundits',                     count:  3 },
    { label: 'fallacy',                     count:  3 },
    { label: 'forthcoming',                 count:  3 },
    { label: 'bearing the brunt',           count:  3 },
    { label: 'aghast',                      count:  3 },
    { label: 'hinge on',                    count:  3 },
    { label: 'braggadocious',              count:  2 },
    { label: 'tantamount to',              count:  2 },
    { label: 'perspicacity',               count:  2 },
    { label: 'ostensibly',                 count:  2 },
    { label: 'unbeknownst',                count:  2 },
    { label: 'linchpin',                   count:  2 },
    { label: 'remiss',                     count:  2 },
    { label: 'conjecture',                 count:  2 },
    { label: 'unabashed',                  count:  2 },
    { label: 'jarring',                    count:  2 },
    { label: 'peel back',                  count:  2 },
    { label: 'buoyant',                    count:  2 },
    { label: 'per se',                     count:  2 },
    { label: 'slander',                    count:  2 },
    { label: 'surreptitiously',            count:  2 },
    { label: 'ebb and flow',               count:  2 },
    { label: 'gander',                     count:  2 },
    { label: 'telltale',                   count:  2 },
    { label: 'zilch',                      count:  2 },
    { label: 'taciturn',                   count:  2 },
    { label: 'fortitude',                  count:  2 },
    { label: 'doldrums',                   count:  2 },
  ].sort((a, b) => b.count - a.count);

  const max = words[0].count;

  function pickColor(count) {
    const t = count / max;
    const arr = t > 0.6 ? palette.high : t > 0.25 ? palette.mid : palette.low;
    return arr[Math.floor(Math.random() * arr.length)];
  }

  function sizeRem(count) {
    return 1.0 + (count / max) * 1.6;
  }

  function render() {
    const canvas = document.getElementById('landing-bubble');
    if (!canvas) return;

    const vw = canvas.parentElement.offsetWidth;
    const vh = Math.min(Math.round(vw * 0.7), 640);
    const dpr = window.devicePixelRatio || 1;

    canvas.width  = vw * dpr;
    canvas.height = vh * dpr;
    canvas.style.width  = vw + 'px';
    canvas.style.height = vh + 'px';

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, vw, vh);

    const pad = 18;
    const placed = [];

    // Use a separate canvas for text measurement (avoids transform issues)
    const mCtx = document.createElement('canvas').getContext('2d');

    function overlaps(x, w, y, h, p, gap) {
      return x < p.x + p.w + gap && x + w + gap > p.x &&
             y < p.y + p.h + gap && y + h + gap > p.y;
    }

    words.forEach(({ label, count }) => {
      const rem = sizeRem(count);
      const px  = rem * 16;
      const fontStr = `500 ${px}px Inter,-apple-system,sans-serif`;
      mCtx.font = fontStr;
      const tw = mCtx.measureText(label).width;
      const w  = tw + pad;
      const h  = px * 1.4;

      let pos = null;

      // Spread words randomly across the full canvas
      for (let attempt = 0; attempt < 800; attempt++) {
        const x = pad + Math.random() * Math.max(0, vw - w - pad * 2);
        const y = pad + Math.random() * Math.max(0, vh - h - pad * 2);
        if (!placed.some(p => overlaps(x, w, y, h, p, pad))) {
          pos = { x, y, w, h };
          break;
        }
      }

      if (!pos) return;
      placed.push(pos);

      ctx.font = fontStr;
      ctx.fillStyle = pickColor(count);
      ctx.fillText(label, pos.x, pos.y + px);
    });
  }

  // Wait for fonts before first render so Inter metrics are correct
  (document.fonts ? document.fonts.ready : Promise.resolve()).then(render);

  let resizeTimer;
  window.addEventListener('resize', () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(render, 200);
  });
})();
