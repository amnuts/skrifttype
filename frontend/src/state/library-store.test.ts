import { describe, it, expect } from 'vitest';
import { LibraryStore } from './library-store';

describe('LibraryStore', () => {
  it('loads fonts and supports multi-select capped at 4', async () => {
    const s = new LibraryStore();
    await s.refresh();
    expect(s.fonts.length).toBeGreaterThan(0);
    const ids = s.fonts.slice(0, 6).map((f) => f.id);
    ids.forEach((id) => s.toggleMulti(id));
    expect(s.selectedIds.length).toBe(4);
  });
  it('filters by search', async () => {
    const s = new LibraryStore();
    s.setSearch('arial'); await s.refresh();
    expect(s.fonts.every((f) => f.family.toLowerCase().includes('arial'))).toBe(true);
  });
});
