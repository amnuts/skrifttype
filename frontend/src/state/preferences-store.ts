export type Theme = 'light' | 'dark';
type Listener = () => void;

export const RIGHT_WIDTH_MIN = 300;
export const RIGHT_WIDTH_MAX = 900;
const RIGHT_WIDTH_DEFAULT = 420;
export const LEFT_WIDTH_MIN = 160;
export const LEFT_WIDTH_MAX = 420;
const LEFT_WIDTH_DEFAULT = 200;
export const LIST_SIZE_MIN = 12;
export const LIST_SIZE_MAX = 32;
const LIST_SIZE_DEFAULT = 17;

export const DEFAULT_PHRASE = 'Typography that breathes.';
const PHRASES_KEY = 'skrifttype.phrases';
const PHRASE_SEL_KEY = 'skrifttype.phraseSelected';

function loadPhrases(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(PHRASES_KEY) ?? '[]');
    return Array.isArray(v) ? v.filter((p): p is string => typeof p === 'string' && p.trim() !== '') : [];
  } catch { return []; }
}

export class PreferencesStore {
  theme: Theme;
  /** Phrases the user has saved for trying fonts out, in addition to the built-in default. */
  phrases: string[] = loadPhrases();
  private selected: string | null = localStorage.getItem(PHRASE_SEL_KEY);
  previewSize = 72;
  /** Width (px) of the right pane; the boundary the user drags. Persisted. */
  rightWidth: number;
  /** Width (px) of the collections sidebar. Persisted. */
  leftWidth: number;
  /** Base font size (px) of the phrase in each library row. Persisted. */
  listSize: number;
  private listeners = new Set<Listener>();

  constructor() {
    const saved = localStorage.getItem('skrifttype.theme') as Theme | null;
    const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false;
    this.theme = saved ?? (prefersDark ? 'dark' : 'light');
    const w = Number(localStorage.getItem('skrifttype.rightWidth'));
    this.rightWidth = Number.isFinite(w) && w > 0 ? this.clampWidth(w) : RIGHT_WIDTH_DEFAULT;
    const lw = Number(localStorage.getItem('skrifttype.leftWidth'));
    this.leftWidth = Number.isFinite(lw) && lw > 0 ? this.clampLeftWidth(lw) : LEFT_WIDTH_DEFAULT;
    const ls = Number(localStorage.getItem('skrifttype.listSize'));
    this.listSize = Number.isFinite(ls) && ls > 0 ? this.clampListSize(ls) : LIST_SIZE_DEFAULT;
    this.apply();
  }
  subscribe(fn: Listener) { this.listeners.add(fn); return () => this.listeners.delete(fn); }
  private emit() { this.listeners.forEach((l) => l()); }
  private apply() { document.documentElement.setAttribute('data-theme', this.theme); }

  toggleTheme() { this.theme = this.theme === 'light' ? 'dark' : 'light'; localStorage.setItem('skrifttype.theme', this.theme); this.apply(); this.emit(); }
  /** The phrase shown in previews: the selected one, else the built-in default. */
  get sampleText(): string {
    if (this.selected !== null && (this.selected === DEFAULT_PHRASE || this.phrases.includes(this.selected))) return this.selected;
    return DEFAULT_PHRASE;
  }
  private savePhrases() {
    localStorage.setItem(PHRASES_KEY, JSON.stringify(this.phrases));
    if (this.selected === null) localStorage.removeItem(PHRASE_SEL_KEY); else localStorage.setItem(PHRASE_SEL_KEY, this.selected);
    this.emit();
  }
  /** Saves a phrase (if new) and makes it the one in use. */
  addPhrase(text: string) {
    const t = text.trim();
    if (!t) return;
    if (t !== DEFAULT_PHRASE && !this.phrases.includes(t)) this.phrases = [...this.phrases, t];
    this.selected = t;
    this.savePhrases();
  }
  selectPhrase(text: string) {
    if (text !== DEFAULT_PHRASE && !this.phrases.includes(text)) return;
    this.selected = text;
    this.savePhrases();
  }
  removePhrase(text: string) {
    this.phrases = this.phrases.filter((p) => p !== text);
    if (this.selected === text) this.selected = null;
    this.savePhrases();
  }
  setPreviewSize(n: number) { this.previewSize = n; this.emit(); }

  clampListSize(n: number) { return Math.max(LIST_SIZE_MIN, Math.min(LIST_SIZE_MAX, Math.round(n))); }
  setListSize(n: number) {
    this.listSize = this.clampListSize(n);
    localStorage.setItem('skrifttype.listSize', String(this.listSize));
    this.emit();
  }
  clampWidth(w: number) { return Math.max(RIGHT_WIDTH_MIN, Math.min(RIGHT_WIDTH_MAX, Math.round(w))); }
  clampLeftWidth(w: number) { return Math.max(LEFT_WIDTH_MIN, Math.min(LEFT_WIDTH_MAX, Math.round(w))); }
  setLeftWidth(w: number) {
    this.leftWidth = this.clampLeftWidth(w);
    localStorage.setItem('skrifttype.leftWidth', String(this.leftWidth));
    this.emit();
  }
  setRightWidth(w: number) {
    this.rightWidth = this.clampWidth(w);
    localStorage.setItem('skrifttype.rightWidth', String(this.rightWidth));
    this.emit();
  }
}
