import type { FontRecord, FontStyle } from '../../data/types';

/** One ConvertFont call: styleId is the face row id of a style. */
export interface PlanCall { styleId: number; allFaces: boolean; label: string; }
/** "family": only this family's faces of a collection; "file": every face in the file(s). */
export type CollectionScope = 'family' | 'file';

const ext = (p: string) => {
  const m = /\.([^.\/]+)$/.exec(p);
  return m ? m[1].toLowerCase() : '';
};
const baseName = (p: string) => p.split(/[\\/]/).pop() || p;
const isWeb = (s: FontStyle) => s.id !== undefined && (ext(s.path) === 'woff' || ext(s.path) === 'woff2');
const isCollection = (s: FontStyle) => s.id !== undefined && (ext(s.path) === 'ttc' || ext(s.path) === 'otc');

export const hasConvertibleStyles = (f: Pick<FontRecord, 'styles'>) => f.styles.some((s) => isWeb(s) || isCollection(s));
export const hasCollectionStyles = (f: Pick<FontRecord, 'styles'>) => f.styles.some(isCollection);

/** Turns a family into the ConvertFont calls to make. Never relies on the family id or format. */
export function planConversion(f: Pick<FontRecord, 'styles'>, scope: CollectionScope): PlanCall[] {
  const calls: PlanCall[] = [];
  const seen = new Set<string>();
  for (const s of f.styles) {
    if (isWeb(s) && !seen.has(s.path)) {
      seen.add(s.path);
      calls.push({ styleId: s.id!, allFaces: false, label: baseName(s.path) });
    }
  }
  for (const s of f.styles) {
    if (!isCollection(s)) continue;
    if (scope === 'file') {
      if (seen.has(s.path)) continue;
      seen.add(s.path);
      calls.push({ styleId: s.id!, allFaces: true, label: baseName(s.path) });
    } else {
      calls.push({ styleId: s.id!, allFaces: false, label: `${s.styleName} (${baseName(s.path)})` });
    }
  }
  return calls;
}
