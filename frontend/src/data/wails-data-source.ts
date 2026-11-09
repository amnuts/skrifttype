import { Events } from '@wailsio/runtime';
import type { DataSource } from './data-source';
import type {
  FontRecord, FontQuery, ActivationState, ActivationDiff, Collection, NewCollection,
  OpenTypeFeature, Range, ReportRequest, ReportData, Settings, WatchedDir, LibraryEventHandlers,
  Classification, FontSourceKind, InstallResult, ConvertResult, ActivationProgress,
} from './types';
import { featuresOf } from './feature-names';
import {
  ActivationService, CollectionService, ConvertService, InstallService, LibraryService, SettingsService,
} from '../../bindings/github.com/amnuts/skrifttype/internal/api';
import type {
  ActivationDiffDTO, FontDTO, FontQueryDTO, InstallResultDTO,
} from '../../bindings/github.com/amnuts/skrifttype/internal/api';

// Go slices marshal to null when empty; normalise at the boundary.
const arr = <T>(v: T[] | null | undefined): T[] => v ?? [];

function toRecord(d: FontDTO): FontRecord {
  return {
    id: d.id,
    family: d.family,
    cssFamily: d.cssFamily,
    classification: (d.classification ?? null) as Classification,
    source: d.source as FontSourceKind,
    styles: arr(d.styles).map((s) => ({
      id: s.id, styleName: s.styleName, weight: s.weight, italic: s.italic,
      path: s.path, url: s.url, available: s.available, activatable: s.activatable,
    })),
    styleCount: d.styleCount,
    designer: d.designer,
    foundry: d.foundry,
    version: d.version,
    format: d.format,
    available: d.available,
    parseError: d.parseError,
    isVariable: d.isVariable,
    gsubFeatures: arr(d.gsubFeatures),
    gposFeatures: arr(d.gposFeatures),
    unicodeRanges: arr(d.unicodeRanges),
    installedByApp: d.installedByApp,
  };
}

const toInstallResult = (r: InstallResultDTO): InstallResult =>
  ({ succeeded: arr(r.succeeded), failed: arr(r.failed) });

function toQuery(q: FontQuery): FontQueryDTO {
  const scope = q.scope ?? 'all';
  return {
    search: q.search ?? '',
    classifications: q.classifications ?? [],
    scope: typeof scope === 'number' ? 'collection' : scope,
    collectionId: typeof scope === 'number' ? scope : 0,
    sort: q.sort ?? 'family',
  };
}

function toDiff(d: ActivationDiffDTO): ActivationDiff {
  const out = { loaded: arr(d.loaded), unloaded: arr(d.unloaded), noop: arr(d.noop), failed: arr(d.failed) };
  if (out.failed.length) console.warn('skrifttype: some fonts could not be activated', out.failed);
  return out;
}

const SETTINGS_KEYS = { theme: 'theme', defaultSampleText: 'default_sample_text', previewSize: 'preview_size' } as const;

// WailsDataSource talks to the Go services bound by main.go. Every method
// is real; MockDataSource is only used outside the desktop webview.
export class WailsDataSource implements DataSource {
  // ---- library ----
  async listFonts(q: FontQuery): Promise<FontRecord[]> {
    return arr(await LibraryService.ListFonts(toQuery(q))).map(toRecord);
  }
  async scopeCounts() {
    const c = await LibraryService.ScopeCounts();
    return { system: c.system, available: c.available, activatable: c.activatable };
  }
  async getFont(id: number): Promise<FontRecord | undefined> {
    const d = await LibraryService.GetFont(id);
    return d ? toRecord(d) : undefined;
  }
  async rescan(): Promise<void> { await LibraryService.Rescan(); }
  isScanning(): Promise<boolean> { return LibraryService.IsScanning(); }
  onLibraryEvents(h: LibraryEventHandlers): () => void {
    const offs = [
      Events.On('library:scanning', (e) => h.scanning?.(e.data)),
      Events.On('library:changed', (e) => h.changed?.(e.data)),
    ];
    return () => offs.forEach((off) => off());
  }

  // ---- activation ----
  onActivationProgress(handler: (p: ActivationProgress) => void): () => void {
    return Events.On('activation:progress', (e) => handler(e.data));
  }
  async getActiveSet(): Promise<ActivationState> {
    const raw = (await ActivationService.GetActiveSet()) ?? {};
    const sources: Record<number, string[]> = {};
    for (const [k, v] of Object.entries(raw)) if (v?.length) sources[Number(k)] = v;
    return { sources };
  }
  async activateManual(id: number) { return toDiff(await ActivationService.ActivateManual(id)); }
  async deactivateManual(id: number) { return toDiff(await ActivationService.DeactivateManual(id)); }
  async activateAllAvailable() { return toDiff(await ActivationService.ActivateAllAvailable()); }
  async deactivateAll() { return toDiff(await ActivationService.DeactivateAll()); }
  async deactivate(id: number) { return toDiff(await ActivationService.Deactivate(id)); }
  async activateCollection(id: number) { return toDiff(await ActivationService.ActivateCollection(id)); }
  async deactivateCollection(id: number) { return toDiff(await ActivationService.DeactivateCollection(id)); }

  // ---- collections ----
  async listCollections(): Promise<Collection[]> {
    return arr(await CollectionService.ListCollections()).map((c) => ({ ...c, fontIds: arr(c.fontIds) }));
  }
  async createCollection(input: NewCollection): Promise<Collection> {
    const c = await CollectionService.CreateCollection(input.name, input.color, input.fontIds ?? []);
    return { ...c, fontIds: arr(c.fontIds) };
  }
  async addToCollection(cid: number, ids: number[]) { await CollectionService.AddToCollection(cid, ids); }
  async removeFromCollection(cid: number, ids: number[]) { await CollectionService.RemoveFromCollection(cid, ids); }
  async renameCollection(cid: number, name: string) { await CollectionService.RenameCollection(cid, name); }
  async setCollectionColor(cid: number, color: string) { await CollectionService.SetCollectionColor(cid, color); }
  async deleteCollection(cid: number) { await CollectionService.DeleteCollection(cid); }

  // ---- system-wide install ----
  canInstallSystemWide(): Promise<boolean> { return InstallService.CanInstallSystemWide(); }
  async installForEveryone(id: number) { return toInstallResult(await InstallService.InstallForEveryone(id)); }
  async uninstallForEveryone(id: number) { return toInstallResult(await InstallService.UninstallForEveryone(id)); }

  // ---- conversion ----
  async convertFont(fontId: number, allFaces: boolean, outDir: string, remember: boolean): Promise<ConvertResult[]> {
    return arr(await ConvertService.ConvertFont(fontId, allFaces, outDir, remember));
  }
  getConvertFolder(): Promise<string> { return ConvertService.GetConvertFolder(); }
  async setConvertFolder(dir: string): Promise<void> { await ConvertService.SetConvertFolder(dir); }
  inWatchedRoot(dir: string): Promise<boolean> { return ConvertService.InWatchedRoot(dir); }

  // ---- preview / glyphs / reports ----
  async getFeatures(id: number): Promise<OpenTypeFeature[]> {
    const f = await this.getFont(id);
    return f ? featuresOf(f) : [];
  }
  async getGlyphRanges(id: number): Promise<Range[]> {
    return arr(await LibraryService.GetGlyphRanges(id));
  }
  async buildReport(input: ReportRequest): Promise<ReportData> {
    const fonts = await this.listFonts({ scope: input.scope });
    return { title: 'skrifttype report', type: input.type, sampleText: input.sampleText, fonts };
  }

  // ---- settings (preferences-backed) ----
  async getSettings(): Promise<Settings> {
    const [theme, sample, size, watchedDirs] = await Promise.all([
      this.getPreference(SETTINGS_KEYS.theme),
      this.getPreference(SETTINGS_KEYS.defaultSampleText),
      this.getPreference(SETTINGS_KEYS.previewSize),
      this.listWatchedRoots(),
    ]);
    return {
      theme: theme === 'light' || theme === 'dark' ? theme : 'system',
      defaultSampleText: sample ?? 'Typography that breathes.',
      previewSize: Number(size) || 72,
      watchedDirs,
    };
  }
  async saveSettings(patch: Partial<Settings>): Promise<Settings> {
    const writes: Promise<void>[] = [];
    if (patch.theme !== undefined) writes.push(this.setPreference(SETTINGS_KEYS.theme, patch.theme));
    if (patch.defaultSampleText !== undefined) writes.push(this.setPreference(SETTINGS_KEYS.defaultSampleText, patch.defaultSampleText));
    if (patch.previewSize !== undefined) writes.push(this.setPreference(SETTINGS_KEYS.previewSize, String(patch.previewSize)));
    await Promise.all(writes);
    return this.getSettings();
  }

  // ---- watched roots & preferences (Go SettingsService) ----
  async listWatchedRoots(): Promise<WatchedDir[]> {
    return arr(await SettingsService.ListWatchedRoots());
  }
  addWatchedRoot(path: string, recursive: boolean): Promise<WatchedDir> {
    return SettingsService.AddWatchedRoot(path, recursive);
  }
  async removeWatchedRoot(id: number): Promise<void> { await SettingsService.RemoveWatchedRoot(id); }
  async setWatchedRootEnabled(id: number, enabled: boolean): Promise<void> {
    await SettingsService.SetWatchedRootEnabled(id, enabled);
  }
  async pickFolder(): Promise<string | null> {
    const p = await SettingsService.PickFolder();
    return p && p.trim() ? p : null;
  }
  async getPreference(key: string): Promise<string | null> {
    const v = await SettingsService.GetPreference(key);
    return v === '' ? null : v;
  }
  async setPreference(key: string, value: string): Promise<void> { await SettingsService.SetPreference(key, value); }
}
