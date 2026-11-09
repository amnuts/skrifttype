import { describe, it, expect, beforeEach } from 'vitest';
import { PreferencesStore, DEFAULT_PHRASE, LIST_SIZE_MAX, LIST_SIZE_MIN } from './preferences-store';

describe('PreferencesStore', () => {
  beforeEach(() => localStorage.clear());
  it('toggles theme and persists', () => {
    const s = new PreferencesStore();
    const start = s.theme;
    s.toggleTheme();
    expect(s.theme).not.toBe(start);
    expect(localStorage.getItem('skrifttype.theme')).toBe(s.theme);
    expect(document.documentElement.getAttribute('data-theme')).toBe(s.theme);
  });
  it('uses the default phrase until the user saves one', () => {
    const s = new PreferencesStore();
    expect(s.sampleText).toBe(DEFAULT_PHRASE);
    let hit = 0; s.subscribe(() => hit++);
    s.addPhrase('  Hi there  ');
    expect(hit).toBe(1);
    expect(s.sampleText).toBe('Hi there');
    expect(new PreferencesStore().sampleText).toBe('Hi there');
  });
  it('clamps and persists the list size', () => {
    const s = new PreferencesStore();
    expect(s.listSize).toBe(17);
    s.setListSize(99); expect(s.listSize).toBe(LIST_SIZE_MAX);
    s.setListSize(1); expect(s.listSize).toBe(LIST_SIZE_MIN);
    s.setListSize(20); expect(new PreferencesStore().listSize).toBe(20);
  });
  it('selects, de-duplicates and removes phrases', () => {
    const s = new PreferencesStore();
    s.addPhrase('One'); s.addPhrase('Two'); s.addPhrase('One');
    expect(s.phrases).toEqual(['One', 'Two']);
    expect(s.sampleText).toBe('One');
    s.selectPhrase('Two'); expect(s.sampleText).toBe('Two');
    s.removePhrase('Two'); expect(s.sampleText).toBe(DEFAULT_PHRASE);
    s.selectPhrase('One'); s.removePhrase('One'); expect(s.sampleText).toBe(DEFAULT_PHRASE);
    s.addPhrase(DEFAULT_PHRASE); expect(s.phrases).toEqual([]);
    s.removePhrase(DEFAULT_PHRASE); expect(s.sampleText).toBe(DEFAULT_PHRASE);
    s.addPhrase('   '); expect(s.phrases).toEqual([]);
  });
});
