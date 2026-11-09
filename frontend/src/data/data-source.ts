import type {
  FontRecord, FontQuery, ActivationState, ActivationDiff, Collection, NewCollection,
  OpenTypeFeature, Range, ReportRequest, ReportData, Settings, WatchedDir, LibraryEventHandlers,
  InstallResult, ConvertResult, ActivationProgress,
} from './types';

export interface DataSource {
  listFonts(query: FontQuery): Promise<FontRecord[]>;
  getFont(id: number): Promise<FontRecord | undefined>;
  /** Family counts of the System and Available scopes, ignoring filters. */
  scopeCounts(): Promise<{ system: number; available: number; activatable: number }>;

  /** Queue a rescan of watched folders and system fonts (fire-and-forget). */
  rescan(): Promise<void>;
  isScanning(): Promise<boolean>;
  /** Subscribe to backend library events; returns an unsubscribe function. */
  onLibraryEvents(handlers: LibraryEventHandlers): () => void;

  /** Subscribe to progress of bulk activation; returns an unsubscribe function. */
  onActivationProgress(handler: (p: ActivationProgress) => void): () => void;
  getActiveSet(): Promise<ActivationState>;
  activateManual(id: number): Promise<ActivationDiff>;
  deactivateManual(id: number): Promise<ActivationDiff>;
  /** Switches on every activatable library font. */
  activateAllAvailable(): Promise<ActivationDiff>;
  /** Switches off every active font, whatever holds it. */
  deactivateAll(): Promise<ActivationDiff>;
  /** Turns a font off outright, even when an active collection holds it. */
  deactivate(id: number): Promise<ActivationDiff>;
  activateCollection(id: number): Promise<ActivationDiff>;
  deactivateCollection(id: number): Promise<ActivationDiff>;

  listCollections(): Promise<Collection[]>;
  createCollection(input: NewCollection): Promise<Collection>;
  addToCollection(cid: number, fontIds: number[]): Promise<void>;
  removeFromCollection(cid: number, fontIds: number[]): Promise<void>;
  renameCollection(cid: number, name: string): Promise<void>;
  setCollectionColor(cid: number, color: string): Promise<void>;
  deleteCollection(cid: number): Promise<void>;

  // System-wide install via the elevated helper (Windows; one UAC prompt each)
  canInstallSystemWide(): Promise<boolean>;
  installForEveryone(id: number): Promise<InstallResult>;
  uninstallForEveryone(id: number): Promise<InstallResult>;

  // On-request conversion of web fonts / collections into plain font files.
  // outDir "" means use the saved conversion folder. Per-face failures are
  // reported in each result's err, not by rejecting.
  convertFont(fontId: number, allFaces: boolean, outDir: string, remember: boolean): Promise<ConvertResult[]>;
  getConvertFolder(): Promise<string>;               // "" when unset
  setConvertFolder(dir: string): Promise<void>;      // "" clears
  inWatchedRoot(dir: string): Promise<boolean>;

  getFeatures(id: number): Promise<OpenTypeFeature[]>;
  getGlyphRanges(id: number): Promise<Range[]>;
  buildReport(input: ReportRequest): Promise<ReportData>;

  getSettings(): Promise<Settings>;
  saveSettings(patch: Partial<Settings>): Promise<Settings>;

  // Watched folders (real backend in the desktop app; localStorage in dev)
  listWatchedRoots(): Promise<WatchedDir[]>;
  addWatchedRoot(path: string, recursive: boolean): Promise<WatchedDir>;
  removeWatchedRoot(id: number): Promise<void>;
  setWatchedRootEnabled(id: number, enabled: boolean): Promise<void>;
  pickFolder(): Promise<string | null>;            // null when cancelled

  // Key/value preferences (server-backed in the desktop app)
  getPreference(key: string): Promise<string | null>;
  setPreference(key: string, value: string): Promise<void>;
}
