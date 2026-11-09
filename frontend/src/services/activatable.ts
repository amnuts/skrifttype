import type { FontRecord } from '../data/types';

export const WEB_FONT_TOOLTIP = "Web fonts can't be activated on Windows. Convert to TTF/OTF to use it.";

/** A family can be activated if any style can; a missing flag (mock/legacy) means yes. */
export function isActivatable(f: Pick<FontRecord, 'styles'>): boolean {
  return f.styles.length === 0 || f.styles.some((s) => s.activatable !== false);
}

/**
 * Installing for all users needs a plain .ttf/.otf file (the elevated helper
 * rejects collections and web fonts). Unloaded styles are assumed installable.
 */
export function hasInstallableStyle(f: Pick<FontRecord, 'styles'>): boolean {
  return f.styles.length === 0 || f.styles.some((s) => /\.(ttf|otf)$/i.test(s.path));
}
