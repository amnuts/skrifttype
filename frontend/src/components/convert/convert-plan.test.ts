import { describe, it, expect } from 'vitest';
import { planConversion, hasConvertibleStyles } from './convert-plan';
import type { FontStyle } from '../../data/types';

const st = (id: number, path: string, styleName = 'Regular'): FontStyle =>
  ({ id, path, styleName, weight: 400, italic: false });
const fam = (...styles: FontStyle[]) => ({ styles });

describe('planConversion', () => {
  it('makes one call per file for a multi-file woff2 family', () => {
    const f = fam(st(1, 'D:/f/a.woff2'), st(2, 'D:/f/b.WOFF2'));
    expect(planConversion(f, 'family')).toEqual([
      { styleId: 1, allFaces: false, label: 'a.woff2' },
      { styleId: 2, allFaces: false, label: 'b.WOFF2' },
    ]);
  });
  it('makes one call per face for a ttc family by default', () => {
    const f = fam(st(5, 'D:/f/c.ttc', 'Regular'), st(6, 'D:/f/c.ttc', 'Bold'));
    const calls = planConversion(f, 'family');
    expect(calls.map((c) => [c.styleId, c.allFaces])).toEqual([[5, false], [6, false]]);
  });
  it('makes one call per unique path for every-face scope', () => {
    const f = fam(st(5, 'D:/f/c.ttc'), st(6, 'D:/f/c.ttc'), st(7, 'D:/f/d.otc'));
    expect(planConversion(f, 'file').map((c) => [c.styleId, c.allFaces])).toEqual([[5, true], [7, true]]);
  });
  it('only converts the web style of a mixed family', () => {
    const f = fam(st(1, 'D:/f/a.ttf'), st(2, 'D:/f/a.woff2'));
    expect(planConversion(f, 'family').map((c) => c.styleId)).toEqual([2]);
  });
  it('dedupes web styles by path', () => {
    const f = fam(st(1, 'D:\\f\\a.woff'), st(2, 'D:\\f\\a.woff'));
    expect(planConversion(f, 'family')).toHaveLength(1);
  });
  it('is empty when nothing is convertible', () => {
    const f = fam(st(1, 'D:/f/a.ttf'), st(2, 'D:/f/b.otf'));
    expect(planConversion(f, 'family')).toEqual([]);
    expect(hasConvertibleStyles(f)).toBe(false);
  });

  it('labels with the file name only for Windows backslash paths', () => {
    const win = 'D:\\fonts\\a.woff';
    expect(planConversion(fam(st(1, win)), 'family')[0].label).toBe('a.woff');
  });
  it('uses the file name in the ttc family-scope label for a backslash path', () => {
    const win = 'C:\\fonts\\c.ttc';
    expect(planConversion(fam(st(5, win, 'Bold')), 'family')[0].label).toBe('Bold (c.ttc)');
  });
});
