// Renders identifiers (column / index / key names) as HTML with <wbr> hints
// after natural break points (_-./:/\) so long names wrap on word boundaries.
//
// Why <wbr> and not a ZWSP (U+200B) in the text node: ZWSPs survive copy-paste
// and silently corrupt SQL queries when users copy a column name out of the
// schema panel. <wbr> is a void element that contributes no character to the
// selection, so copy yields the original identifier verbatim.

const SEPARATORS = /([_\-.:/\\])/g

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch)
}

export function softBreakIdentifierHtml(value: string | null | undefined): string {
  const raw = String(value ?? '')
  if (!raw) return raw
  return escapeHtml(raw).replace(SEPARATORS, '$1<wbr>')
}

export function softBreakIdentifierListHtml(values: ReadonlyArray<string>): string {
  return values.map((v) => softBreakIdentifierHtml(v)).join(', ')
}
