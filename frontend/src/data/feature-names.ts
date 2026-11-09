import type { FontRecord, OpenTypeFeature } from './types';

// Human-readable names for common OpenType feature tags. Unknown tags fall
// back to the tag itself, upper-cased; ssNN/cvNN are generated.
const FEATURE_NAMES: Record<string, string> = {
  aalt: 'Access All Alternates', c2sc: 'Small Capitals From Capitals', calt: 'Contextual Alternates',
  case: 'Case-Sensitive Forms', ccmp: 'Glyph Composition', clig: 'Contextual Ligatures',
  cpsp: 'Capital Spacing', curs: 'Cursive Positioning', dlig: 'Discretionary Ligatures',
  dnom: 'Denominators', frac: 'Fractions', hist: 'Historical Forms', kern: 'Kerning',
  liga: 'Standard Ligatures', lnum: 'Lining Figures', locl: 'Localized Forms',
  mark: 'Mark Positioning', mkmk: 'Mark-to-Mark Positioning', numr: 'Numerators',
  onum: 'Oldstyle Figures', ordn: 'Ordinals', pnum: 'Proportional Figures', salt: 'Stylistic Alternates',
  sinf: 'Scientific Inferiors', smcp: 'Small Capitals', subs: 'Subscript', sups: 'Superscript',
  swsh: 'Swashes', titl: 'Titling', tnum: 'Tabular Figures', zero: 'Slashed Zero',
};

export function featureName(tag: string): string {
  const known = FEATURE_NAMES[tag];
  if (known) return known;
  const set = /^ss(\d\d)$/.exec(tag);
  if (set) return `Stylistic Set ${Number(set[1])}`;
  const cv = /^cv(\d\d)$/.exec(tag);
  if (cv) return `Character Variant ${Number(cv[1])}`;
  return tag.toUpperCase();
}

export function featuresOf(f: Pick<FontRecord, 'gsubFeatures' | 'gposFeatures'>): OpenTypeFeature[] {
  return [
    ...f.gsubFeatures.map((tag) => ({ tag, name: featureName(tag), kind: 'gsub' as const })),
    ...f.gposFeatures.map((tag) => ({ tag, name: featureName(tag), kind: 'gpos' as const })),
  ];
}
