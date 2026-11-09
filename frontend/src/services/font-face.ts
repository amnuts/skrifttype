import type { FontRecord } from '../data/types';

/**
 * Resolves the CSS font-family value for a record.
 *
 * Records from the real backend carry a per-style `url` (served by Go at
 * /fontfile/<id>) and a synthetic `cssFamily` ("skrift-<id>"): each style is
 * registered once with the FontFace API under that family, so previews work
 * for library fonts that aren't installed. The browser fetches a face only
 * when text first uses it. Mock records have no urls and render by their
 * installed family name. A generic fallback by classification keeps previews
 * legible while a face loads or if it fails.
 */
const FALLBACK: Record<string, string> = {
  serif: 'Georgia, "Times New Roman", serif',
  sans: '"Segoe UI", system-ui, sans-serif',
  mono: 'Consolas, ui-monospace, monospace',
  script: '"Segoe Script", cursive',
  display: '"Arial Black", sans-serif',
};

type FaceSource = Pick<FontRecord, 'cssFamily' | 'classification' | 'styles'>;

// Keyed per style (family + url) so faces added by a later rescan still get
// registered.
const registered = new Map<string, FontFace>();

function ensureRegistered(f: FaceSource) {
  if (typeof FontFace === 'undefined' || !document.fonts) return;
  for (const s of f.styles) {
    if (!s.url || s.available === false) continue;
    const key = `${f.cssFamily}|${s.url}`;
    if (registered.has(key)) continue;
    try {
      const face = new FontFace(f.cssFamily, `url("${s.url}")`, {
        weight: String(s.weight),
        style: s.italic ? 'italic' : 'normal',
        display: 'block',
      });
      registered.set(key, face);
      document.fonts.add(face);
    } catch (err) {
      console.warn('skrifttype: could not register font face', f.cssFamily, s.url, err);
    }
  }
}

export function cssFamilyFor(f: FaceSource): string {
  ensureRegistered(f);
  const fb = FALLBACK[f.classification ?? 'sans'] ?? 'sans-serif';
  return `"${f.cssFamily}", ${fb}`;
}

// Size multipliers per family + text, measured once.
const scaleCache = new Map<string, Promise<number>>();
const TARGET_X = 0.5;
/** Line box height as a multiple of the base size; the ink must fit inside it. */
export const ROW_LINE = 1.3;
let measureCtx: CanvasRenderingContext2D | null | undefined;

/**
 * Size multiplier for a family's list text. Matches x-height across families so
 * faces that look very large or small read alike, then shrinks further if the
 * text's ink (tall swashes, deep descenders) would spill past the fixed-height
 * line box and be clipped. Resolves to 1 when the face can't be loaded or measured.
 */
export function fitScale(f: FaceSource, text: string): Promise<number> {
  const ck = `${f.cssFamily}|${text}`;
  const hit = scaleCache.get(ck);
  if (hit) return hit;
  const p = (async () => {
    try {
      if (typeof document === 'undefined' || !document.fonts || typeof FontFace === 'undefined') return 1;
      cssFamilyFor(f);
      if (!f.styles.some((s) => s.url && s.available !== false)) return 1;
      const spec = `100px "${f.cssFamily}"`;
      await document.fonts.load(spec, `x${text}`);
      if (!document.fonts.check(spec, 'x')) return 1;
      measureCtx ??= document.createElement('canvas').getContext('2d');
      if (!measureCtx) return 1;
      measureCtx.font = spec;
      const xh = measureCtx.measureText('x').actualBoundingBoxAscent / 100;
      let k = xh > 0.05 ? Math.min(1.5, Math.max(0.65, TARGET_X / xh)) : 1;
      // Fit the ink inside the line box: its baseline sits at half the line
      // height plus half the font's ascent/descent imbalance.
      const m = measureCtx.measureText(text);
      const fa = m.fontBoundingBoxAscent / 100; const fd = m.fontBoundingBoxDescent / 100;
      const ia = m.actualBoundingBoxAscent / 100; const id = m.actualBoundingBoxDescent / 100;
      if ([fa, fd, ia, id].every(Number.isFinite)) {
        const half = ROW_LINE / 2; const skew = (fa - fd) / 2;
        const room = (budget: number, need: number) => (need > 0.01 && budget > 0 ? budget / need : Infinity);
        k = Math.min(k, room(half + skew, ia), room(half - skew, id));
      }
      return Math.max(0.4, k);
    } catch {
      return 1;
    }
  })();
  scaleCache.set(ck, p);
  return p;
}

type StyleOf = FaceSource['styles'][number];

// One FontFace per style under its own family name, so a style can be shown
// even when it shares weight/italic with a sibling (legacy "Expanded",
// "Condensed" or "Light" cuts inside one family).
const styleRegistered = new Set<string>();

/**
 * CSS declarations (font-family, weight, style) that show exactly one style
 * of a family. Styles without a served url (mock data) fall back to the
 * family name plus weight and italic.
 */
export function cssForStyle(f: FaceSource, s: StyleOf): string {
  const fb = FALLBACK[f.classification ?? 'sans'] ?? 'sans-serif';
  const weight = `font-weight:${s.weight};font-style:${s.italic ? 'italic' : 'normal'}`;
  if (!s.url || s.available === false || typeof FontFace === 'undefined' || !document.fonts || s.id === undefined) {
    return `font-family:${cssFamilyFor(f)};${weight}`;
  }
  const family = `${f.cssFamily}-s${s.id}`;
  const key = `${family}|${s.url}`;
  if (!styleRegistered.has(key)) {
    styleRegistered.add(key);
    try {
      document.fonts.add(new FontFace(family, `url("${s.url}")`, {
        weight: String(s.weight),
        style: s.italic ? 'italic' : 'normal',
        display: 'block',
      }));
    } catch (err) {
      console.warn('skrifttype: could not register font style', family, s.url, err);
    }
  }
  return `font-family:"${family}", ${fb};${weight}`;
}

/** The face a report sample shows: the upright style closest to regular weight. */
function reportStyle(f: FaceSource) {
  const usable = f.styles.filter((s) => s.url && s.available !== false);
  const rank = (s: FaceSource['styles'][number]) => Math.abs(s.weight - 400) + (s.italic ? 1000 : 0);
  return usable.sort((a, b) => rank(a) - rank(b))[0];
}

/**
 * Makes the fonts usable in a standalone document (the report window).
 * That window doesn't share this document's FontFaceSet, and fetching font
 * URLs again from it (or via fetch) doesn't reliably reach the asset server
 * in dev, so the FontFace objects registered here are loaded and handed to
 * the other document's set directly. One face per family: the one the
 * report samples render. Returns a count of faces added and failed.
 */
export async function shareFontFaces(fonts: FaceSource[], target: Document, onError: (msg: string) => void = () => {}) {
  const queue = fonts.slice();
  const faceFor = (f: FaceSource) => {
    ensureRegistered(f);
    const s = reportStyle(f);
    return s?.url ? { key: `${f.cssFamily}|${s.url}`, face: registered.get(`${f.cssFamily}|${s.url}`) } : undefined;
  };
  const loadOne = async (f: FaceSource) => {
    let hit = faceFor(f);
    if (!hit?.face) return onError(`${f.cssFamily}: no font file`);
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        await hit.face.load();
        target.fonts.add(hit.face);
        return;
      } catch (err) {
        if (attempt === 1) return onError(`${f.cssFamily}: ${String(err)}`);
        // A FontFace that failed stays failed; re-register a fresh one to retry.
        document.fonts.delete(hit.face);
        registered.delete(hit.key);
        hit = faceFor(f);
        if (!hit?.face) return onError(`${f.cssFamily}: ${String(err)}`);
      }
    }
  };
  // Bounded concurrency: hundreds of simultaneous font requests get dropped.
  await Promise.all(Array.from({ length: 12 }, async () => {
    for (let f = queue.shift(); f; f = queue.shift()) await loadOne(f);
  }));
}
