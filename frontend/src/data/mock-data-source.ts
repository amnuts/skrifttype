import type { DataSource } from './data-source';
import type {
  FontRecord, FontQuery, ActivationState, ActivationDiff, Collection, NewCollection,
  OpenTypeFeature, Range, ReportRequest, ReportData, Settings, LibraryEventHandlers, InstallResult, ConvertResult,
} from './types';
import raw from '../mock/fonts.generated.json';
import { SEED_COLLECTIONS } from '../mock/seed-collections';
import { featuresOf } from './feature-names';

const within = (dir: string, root: string) => {
  const norm = (p: string) => p.replace(/[\\/]+/g, '/').replace(/\/$/, '').toLowerCase();
  const d = norm(dir), r = norm(root);
  return d === r || d.startsWith(r + '/');
};
const delay = <T>(v: T, ms = 60) => new Promise<T>((r) => setTimeout(() => r(v), ms));

export class MockDataSource implements DataSource {
  private fonts: FontRecord[] = (raw as FontRecord[]).map((f) => ({ ...f }));
  private collections: Collection[];
  private sources: Record<number, string[]> = {};
  private settings: Settings = {
    theme: 'system',
    defaultSampleText: 'Typography that breathes.',
    previewSize: 72,
    watchedDirs: [],
  };

  constructor() {
    // Seed collections from the user's *library* (toggleable) fonts only, so the
    // active set lines up with rows the user can actually toggle.
    const lib = this.fonts.filter((f) => f.source === 'library');
    const byClass = (c: string, n: number) =>
      lib.filter((f) => f.classification === c).slice(0, n).map((f) => f.id);
    this.collections = SEED_COLLECTIONS.map((c) => ({
      ...c,
      fontIds: c.id === 1 ? byClass('sans', 12).concat(byClass('mono', 4))
             : c.id === 2 ? byClass('serif', 10).concat(byClass('mono', 6))
             : byClass('display', 6).concat(byClass('script', 4)),
    }));
    // Apply initially-active collections + a few manual actives (library fonts).
    for (const col of this.collections) if (col.active) this.addSource(col.fontIds, `collection:${col.id}`);
    this.addSource(lib.slice(0, 4).map((f) => f.id), 'manual');
  }

  private addSource(ids: number[], key: string) {
    for (const id of ids) { (this.sources[id] ??= []); if (!this.sources[id].includes(key)) this.sources[id].push(key); }
  }
  private removeSource(ids: number[], key: string) {
    for (const id of ids) { if (this.sources[id]) this.sources[id] = this.sources[id].filter((k) => k !== key); if (this.sources[id]?.length === 0) delete this.sources[id]; }
  }
  private isActive = (id: number) => (this.sources[id]?.length ?? 0) > 0;

  /** Compute a diff given the before/after active sets. */
  private diff(before: Set<number>, after: Set<number>): ActivationDiff {
    const loaded = [...after].filter((x) => !before.has(x));
    const unloaded = [...before].filter((x) => !after.has(x));
    return { loaded, unloaded, noop: [] };
  }
  private snapshot = () => new Set(this.fonts.filter((f) => this.isActive(f.id)).map((f) => f.id));

  async scopeCounts() {
    const available = this.fonts.filter((f) => f.source === 'library').length;
    return delay({ system: this.fonts.length - available, available, activatable: available });
  }

  async listFonts(query: FontQuery): Promise<FontRecord[]> {
    let out = this.fonts;
    if (query.scope === 'system') out = out.filter((f) => f.source !== 'library');
    else if (query.scope === 'all') out = out.filter((f) => f.source === 'library');
    else if (query.scope === 'active') out = out.filter((f) => this.isActive(f.id));
    else if (typeof query.scope === 'number') {
      const col = this.collections.find((c) => c.id === query.scope);
      const ids = new Set(col?.fontIds ?? []);
      out = out.filter((f) => ids.has(f.id));
    }
    if (query.classifications?.length) out = out.filter((f) => f.classification && query.classifications!.includes(f.classification as any));
    if (query.search?.trim()) {
      const q = query.search.toLowerCase();
      out = out.filter((f) => f.family.toLowerCase().includes(q) || (f.designer ?? '').toLowerCase().includes(q));
    }
    out = [...out].sort((a, b) => query.sort === 'styles' ? b.styleCount - a.styleCount : a.family.localeCompare(b.family));
    return delay(out);
  }
  async getFont(id: number) { return delay(this.fonts.find((f) => f.id === id)); }

  async rescan() { return delay(undefined); }
  async isScanning() { return delay(false); }
  onLibraryEvents(_handlers: LibraryEventHandlers) { return () => {}; }

  onActivationProgress(): () => void { return () => {}; }
  async getActiveSet(): Promise<ActivationState> { return delay({ sources: structuredClone(this.sources) }); }
  async activateManual(id: number) { const b = this.snapshot(); this.addSource([id], 'manual'); return delay(this.diff(b, this.snapshot())); }
  async deactivateManual(id: number) { const b = this.snapshot(); this.removeSource([id], 'manual'); return delay(this.diff(b, this.snapshot())); }
  async activateAllAvailable() {
    const b = this.snapshot();
    this.addSource(this.fonts.filter((f) => f.source === 'library').map((f) => f.id), 'manual');
    return delay(this.diff(b, this.snapshot()));
  }
  async deactivateAll() {
    const b = this.snapshot();
    for (const k of Object.keys(this.sources)) delete this.sources[Number(k)];
    return delay(this.diff(b, this.snapshot()));
  }
  async deactivate(id: number) {
    const b = this.snapshot();
    this.removeSource([id], 'manual');
    for (const c of this.collections) this.removeSource([id], `collection:${c.id}`);
    return delay(this.diff(b, this.snapshot()));
  }
  async activateCollection(id: number) {
    const b = this.snapshot(); const col = this.collections.find((c) => c.id === id);
    if (col) { col.active = true; this.addSource(col.fontIds, `collection:${id}`); }
    return delay(this.diff(b, this.snapshot()));
  }
  async deactivateCollection(id: number) {
    const b = this.snapshot(); const col = this.collections.find((c) => c.id === id);
    if (col) { col.active = false; this.removeSource(col.fontIds, `collection:${id}`); }
    return delay(this.diff(b, this.snapshot()));
  }

  async listCollections() { return delay(structuredClone(this.collections)); }
  async createCollection(input: NewCollection) {
    const col: Collection = { id: Math.max(0, ...this.collections.map((c) => c.id)) + 1, name: input.name, color: input.color, fontIds: input.fontIds ?? [], active: false };
    this.collections.push(col); return delay(col);
  }
  async addToCollection(cid: number, fontIds: number[]) {
    const col = this.collections.find((c) => c.id === cid);
    if (col) { col.fontIds = [...new Set([...col.fontIds, ...fontIds])]; if (col.active) this.addSource(fontIds, `collection:${cid}`); }
    return delay(undefined);
  }
  async removeFromCollection(cid: number, fontIds: number[]) {
    const col = this.collections.find((c) => c.id === cid);
    if (col) { col.fontIds = col.fontIds.filter((id) => !fontIds.includes(id)); if (col.active) this.removeSource(fontIds, `collection:${cid}`); }
    return delay(undefined);
  }
  async renameCollection(cid: number, name: string) {
    const col = this.collections.find((c) => c.id === cid);
    if (col) col.name = name;
    return delay(undefined);
  }
  async setCollectionColor(cid: number, color: string) {
    const col = this.collections.find((c) => c.id === cid);
    if (col) col.color = color;
    return delay(undefined);
  }
  async deleteCollection(cid: number) {
    const col = this.collections.find((c) => c.id === cid);
    if (col) { this.removeSource(col.fontIds, `collection:${cid}`); this.collections = this.collections.filter((c) => c.id !== cid); }
    return delay(undefined);
  }

  async canInstallSystemWide() { return delay(false); }
  async installForEveryone(_id: number): Promise<InstallResult> { throw new Error('Not available in the browser preview'); }
  async uninstallForEveryone(_id: number): Promise<InstallResult> { throw new Error('Not available in the browser preview'); }

  async getFeatures(id: number): Promise<OpenTypeFeature[]> {
    const f = this.fonts.find((x) => x.id === id);
    return delay(f ? featuresOf(f) : []);
  }
  async getGlyphRanges(id: number): Promise<Range[]> { return delay(this.fonts.find((f) => f.id === id)?.unicodeRanges ?? []); }
  async buildReport(input: ReportRequest): Promise<ReportData> {
    const fonts = await this.listFonts({ scope: input.scope });
    return delay({ title: 'skrifttype report', type: input.type, sampleText: input.sampleText, fonts });
  }

  async getSettings() { return delay(structuredClone(this.settings)); }
  async saveSettings(patch: Partial<Settings>) { this.settings = { ...this.settings, ...patch }; return delay(structuredClone(this.settings)); }

  private readonly WR_KEY = 'skrifttype.mock.watchedRoots';

  private readRoots(): import('./types').WatchedDir[] {
    try { return JSON.parse(localStorage.getItem(this.WR_KEY) ?? '[]'); } catch { return []; }
  }
  private writeRoots(rows: import('./types').WatchedDir[]) {
    localStorage.setItem(this.WR_KEY, JSON.stringify(rows));
  }

  async listWatchedRoots() { return delay(this.readRoots()); }
  async addWatchedRoot(path: string, recursive: boolean) {
    const rows = this.readRoots();
    const row = { id: Math.max(0, ...rows.map((r) => r.id)) + 1, path, recursive, enabled: true };
    rows.push(row); this.writeRoots(rows);
    return delay(row);
  }
  async removeWatchedRoot(id: number) {
    this.writeRoots(this.readRoots().filter((r) => r.id !== id));
    return delay(undefined);
  }
  async setWatchedRootEnabled(id: number, enabled: boolean) {
    this.writeRoots(this.readRoots().map((r) => r.id === id ? { ...r, enabled } : r));
    return delay(undefined);
  }
  async pickFolder() {
    const p = window.prompt('Folder path (dev stand-in for the native picker):', 'C:\\\\Fonts');
    return delay(p && p.trim() ? p.trim() : null);
  }
  async convertFont(fontId: number, allFaces: boolean, outDir: string, remember: boolean): Promise<ConvertResult[]> {
    const f = this.fonts.find((x) => x.id === fontId || x.styles.some((s) => s.id === fontId));
    const dir = outDir || localStorage.getItem('skrifttype.convertFolder') || '';
    if (!f || !dir) return delay([{ faceIndex: 0, path: '', err: 'no conversion folder' }]);
    if (remember) localStorage.setItem('skrifttype.convertFolder', dir);
    const n = allFaces ? Math.max(1, f.styles.length) : 1;
    return delay(Array.from({ length: n }, (_, i) => ({ faceIndex: i, path: `${dir.replace(/[\\/]+$/, '')}/${f.family}-${i}.ttf`, err: '' })));
  }
  async getConvertFolder() { return delay(localStorage.getItem('skrifttype.convertFolder') ?? ''); }
  async setConvertFolder(dir: string) {
    if (dir) localStorage.setItem('skrifttype.convertFolder', dir); else localStorage.removeItem('skrifttype.convertFolder');
    return delay(undefined);
  }
  async inWatchedRoot(dir: string) {
    return delay(this.readRoots().some((r) => r.enabled && within(dir, r.path)));
  }
  async getPreference(key: string) {
    return delay(localStorage.getItem(`skrifttype.pref.${key}`));
  }
  async setPreference(key: string, value: string) {
    localStorage.setItem(`skrifttype.pref.${key}`, value);
    return delay(undefined);
  }
}
