/* Structured phrase presentation. Match literal text, longest first on overlaps;
 * annotate each expression once and emphasize later occurrences without repeating its gloss. */
(function (root) {
  function escape(value) {
    return String(value || '').replace(/&/g, '&amp;').replace(/</g, '&lt;')
      .replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }
  function key(words) {
    return [...new Set((words || []).map(w => w.canonical.trim().toLowerCase()))].sort().join(' · ');
  }
  function format(phrase, words) {
    const matches = [];
    (words || []).forEach((w, index) => {
      if (!w.text) return;
      const literal = w.text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const re = new RegExp(literal, 'giu');
      for (const m of phrase.matchAll(re)) {
        const start = m.index, end = start + m[0].length;
        const letter = /[\p{L}\p{N}_]/u;
        if ((start && letter.test(phrase[start - 1]) && letter.test(m[0][0])) ||
            (end < phrase.length && letter.test(phrase[end]) && letter.test(m[0].slice(-1)))) continue;
        matches.push({start, end, w, index});
      }
    });
    matches.sort((a, b) => a.start - b.start || b.end - a.end || a.index - b.index);
    let result = '', position = 0;
    const seen = new Set();
    for (const m of matches) {
      if (m.start < position) continue;
      result += escape(phrase.slice(position, m.start)) + '<strong>' + escape(phrase.slice(m.start, m.end)) + '</strong>';
      if (!seen.has(m.index) && m.w.meaning) result += ' <span class="def">(' + escape(m.w.meaning) + ')</span>';
      seen.add(m.index);
      position = m.end;
    }
    result += escape(phrase.slice(position));
    // Never lose a gloss if an expression is absent or wholly overlapped.
    (words || []).forEach((w, i) => { if (!seen.has(i)) result += ' <span class="def">(' + escape(w.text) + ': ' + escape(w.meaning) + ')</span>'; });
    return result;
  }
  root.PhraselyHeadwords = {format, key};
  if (typeof module !== 'undefined') module.exports = root.PhraselyHeadwords;
})(globalThis);
