import { describe, it, expect } from 'vitest';
import { isActivatable, hasInstallableStyle } from './activatable';
import type { FontRecord, FontStyle } from '../data/types';

const st = (activatable?: boolean): FontStyle =>
  ({ styleName: 'Regular', weight: 400, italic: false, path: 'x', activatable });
const rec = (styles: FontStyle[]) => ({ styles } as FontRecord);

describe('isActivatable', () => {
  it('is false when every style is a web font', () => {
    expect(isActivatable(rec([st(false), st(false)]))).toBe(false);
  });
  it('is true when at least one style is activatable', () => {
    expect(isActivatable(rec([st(false), st(true)]))).toBe(true);
  });
  it('treats a missing flag (mock/legacy) as activatable', () => {
    expect(isActivatable(rec([st()]))).toBe(true);
  });
  it('is true when styles are not loaded', () => {
    expect(isActivatable(rec([]))).toBe(true);
  });
});

describe('hasInstallableStyle', () => {
  const at = (path: string): FontStyle =>
    ({ styleName: 'Regular', weight: 400, italic: false, path });
  it('is true for ttf/otf paths, case-insensitively', () => {
    expect(hasInstallableStyle(rec([at('C:\f\A.TTF')]))).toBe(true);
    expect(hasInstallableStyle(rec([at('/f/a.otf')]))).toBe(true);
  });
  it('is false for collections and web fonts only', () => {
    expect(hasInstallableStyle(rec([at('/f/a.ttc'), at('/f/b.OTC'), at('/f/c.woff2')]))).toBe(false);
  });
  it('is true when a collection family also has a ttf', () => {
    expect(hasInstallableStyle(rec([at('/f/a.ttc'), at('/f/b.ttf')]))).toBe(true);
  });
  it('is true when styles are not loaded', () => {
    expect(hasInstallableStyle(rec([]))).toBe(true);
  });
});
