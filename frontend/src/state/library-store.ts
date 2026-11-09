import { dataSource } from '../data';
import type { FontRecord, FilterClass, FontQuery } from '../data/types';

type Listener = () => void;
export type Scope = 'all' | 'system' | 'active' | number;

export class LibraryStore {
  fonts: FontRecord[] = [];
  /** Family counts for the System and Available scopes (unfiltered). */
  scopeCounts = { system: 0, available: 0, activatable: 0 };
  loading = false;
  search = '';
  classifications: FilterClass[] = [];
  scope: Scope = 'all';
  selectedIds: number[] = [];     // first entry = primary selection
  private listeners = new Set<Listener>();

  subscribe(fn: Listener) { this.listeners.add(fn); return () => this.listeners.delete(fn); }
  private emit() { this.listeners.forEach((l) => l()); }

  private query(): FontQuery { return { search: this.search, classifications: this.classifications, scope: this.scope, sort: 'family' }; }

  async refresh() {
    this.loading = true; this.emit();
    const [fonts, counts] = await Promise.all([dataSource.listFonts(this.query()), dataSource.scopeCounts()]);
    this.fonts = fonts;
    this.scopeCounts = counts;
    // Drop selections no longer in view.
    const ids = new Set(this.fonts.map((f) => f.id));
    this.selectedIds = this.selectedIds.filter((id) => ids.has(id));
    this.loading = false; this.emit();
  }

  setSearch(s: string) { this.search = s; this.refresh(); }
  setScope(s: Scope) { this.scope = s; this.refresh(); }
  toggleClass(c: FilterClass) {
    this.classifications = this.classifications.includes(c)
      ? this.classifications.filter((x) => x !== c) : [...this.classifications, c];
    this.refresh();
  }

  get primary(): FontRecord | undefined { return this.fonts.find((f) => f.id === this.selectedIds[0]); }
  selectSingle(id: number) { this.selectedIds = [id]; this.emit(); }
  toggleMulti(id: number) {
    this.selectedIds = this.selectedIds.includes(id)
      ? this.selectedIds.filter((x) => x !== id) : [...this.selectedIds, id].slice(-4);
    this.emit();
  }
}
