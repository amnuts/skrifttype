export const codepointHex = (cp: number) => 'U+' + cp.toString(16).toUpperCase().padStart(4, '0');
// Browsers apply standard ligatures by default, so switching `liga` off has to
// be said explicitly; every other feature is off unless listed.
export const featureSettings = (tags: Iterable<string>) => {
  const on = new Set(tags);
  const parts = [...on].map((t) => `"${t}" 1`);
  if (!on.has('liga')) parts.push('"liga" 0');
  return parts.join(', ');
};
export const styleCountLabel = (n: number) => `${n} ${n === 1 ? 'style' : 'styles'}`;

const fileTokens = (path: string) =>
  (path.split(/[\\/]/).pop() ?? path).replace(/\.[^.]+$/, '').split(/[\s_-]+/).filter(Boolean);

/**
 * Pill labels for a family's styles. Files that share a style name (e.g. the
 * Nerd Font "Mono" and proportional builds of one weight) get the words that
 * set their file names apart appended, so the pills can be told apart.
 */
export function styleLabels(styles: ReadonlyArray<{ styleName: string; path: string }>): string[] {
  const base = styles.map((s) => s.styleName || 'Regular');
  const groups = new Map<string, number[]>();
  base.forEach((b, i) => groups.set(b, [...(groups.get(b) ?? []), i]));
  const out = [...base];
  for (const idx of groups.values()) {
    if (idx.length < 2) continue;
    const toks = idx.map((i) => fileTokens(styles[i].path));
    // Words every file has (as many times as the least-repeating file has them) are shared.
    const count = (ts: string[], t: string) => ts.filter((x) => x === t).length;
    const shared = (t: string) => Math.min(...toks.map((ts) => count(ts, t)));
    const extras = toks.map((ts) => {
      const seen = new Map<string, number>();
      return ts.filter((t) => {
        const n = (seen.get(t) ?? 0) + 1;
        seen.set(t, n);
        return n > shared(t);
      }).join(' ');
    });
    const distinct = new Set(extras).size === extras.length;
    idx.forEach((i, n) => {
      if (distinct) out[i] = extras[n] ? `${base[i]} · ${extras[n]}` : base[i];
      else out[i] = `${base[i]} · ${n + 1}`;
    });
  }
  return out;
}
