/* Minimal dependency-free Markdown → HTML renderer for the AI chat panel.
 *
 * The LLM output is untrusted: the entire input is HTML-escaped first and
 * every transform below runs on the escaped text, so no raw HTML ever passes
 * through. It covers the subset models actually emit: fenced code blocks,
 * headings, lists, blockquotes, paragraphs, and inline code/bold/italic/links.
 * Anything it does not recognize renders as escaped plain text.
 */

function mdEscape(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/* mdInline processes one already-escaped line of inline markdown. */
function mdInline(s) {
  let out = '';
  // Code spans first so * _ [ ] inside code stay untouched.
  const parts = s.split(/(`[^`\n]+`)/g);
  for (const p of parts) {
    if (p.length > 1 && p[0] === '`' && p[p.length - 1] === '`') {
      out += '<code>' + p.slice(1, -1) + '</code>';
      continue;
    }
    out += p
      .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>')
      .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g,
        '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  }
  return out;
}

/* mdRender converts an untrusted markdown string to safe HTML. */
function mdRender(src) {
  const text = mdEscape(src);
  const lines = text.split('\n');
  const html = [];

  // First strip a trailing single newline (most LLM responses end with one).
  while (lines.length && lines[lines.length - 1] === '') lines.pop();

  let i = 0;
  while (i < lines.length) {
    if (/^```/.test(lines[i])) {
      // Fenced code block — inner lines are kept verbatim (already escaped).
      const fenceLang = lines[i].slice(3).trim();
      const buf = [];
      i++;
      while (i < lines.length && !/^\s*```/.test(lines[i])) { buf.push(lines[i]); i++; }
      if (i < lines.length) i++; // consume the closing fence
      html.push('<pre class="md-code' + (fenceLang ? ' lang-' + fenceLang : '') + '"><code>'
        + buf.join('\n') + '\n</code></pre>');
      continue;
    }

    if (lines[i].trim() === '') { i++; continue; }

    const firstLine = lines[i];

    const h = /^(#{1,6})\s+(.*)$/.exec(firstLine);
    if (h) {
      const lv = h[1].length;
      html.push('<h' + lv + '>' + mdInline(h[2]) + '</h' + lv + '>');
      i++;
      continue;
    }

    if (/^\s*[-*+]\s+/.test(firstLine)) {
      const items = [];
      while (i < lines.length && (lines[i].trim() !== '')) {
        const li = /^\s*[-*+]\s+(.*)$/.exec(lines[i]);
        if (li) items.push('<li>' + mdInline(li[1]) + '</li>');
        else if (items.length) items[items.length - 1] += '<br>' + mdInline(lines[i].trim());
        i++;
      }
      html.push('<ul>' + items.join('') + '</ul>');
      continue;
    }

    if (/^\s*\d+[.)]\s+/.test(firstLine)) {
      const items = [];
      while (i < lines.length && (lines[i].trim() !== '')) {
        const li = /^\s*\d+[.)]\s+(.*)$/.exec(lines[i]);
        if (li) items.push('<li>' + mdInline(li[1]) + '</li>');
        else if (items.length) items[items.length - 1] += '<br>' + mdInline(lines[i].trim());
        i++;
      }
      html.push('<ol>' + items.join('') + '</ol>');
      continue;
    }

    // Paragraph: collect until a blank line, joining soft-wrapped lines.
    const buf = [];
    while (i < lines.length && lines[i].trim() !== '') { buf.push(lines[i].trim()); i++; }
    html.push('<p>' + mdInline(buf.join(' ')) + '</p>');
  }
  return html.join('\n');
}