export type Classification = 'serif' | 'sans' | 'mono' | 'script' | 'display' | null;
export type FilterClass = Exclude<Classification, null>;
export type FontSourceKind = 'library' | 'system-hkcu' | 'system-hklm';

export interface FontStyle {
  styleName: string;          // "Regular", "Bold Italic"
  weight: number;             // 100..900
  italic: boolean;
  path: string;
  id?: number;                // face id (real backend)
  url?: string;               // served font file; previews register it via FontFace
  available?: boolean;        // false when the file has gone missing
  activatable?: boolean;      // false for web fonts (woff/woff2); undefined = activatable
}

export interface Range { start: number; end: number; } // inclusive codepoints

export interface FontRecord {
  id: number;
  family: string;
  cssFamily: string;          // value to use in CSS font-family
  classification: Classification;
  source: FontSourceKind;
  styles: FontStyle[];
  styleCount: number;
  designer?: string | null;
  foundry?: string | null;
  version?: string | null;
  format: string;             // 'ttf' | 'otf' | 'ttc' | 'otc' | 'woff' | 'woff2'
  available: boolean;
  parseError?: string | null;
  isVariable: boolean;
  gsubFeatures: string[];     // ['liga','dlig','smcp',...]
  gposFeatures: string[];     // ['kern','mark',...]
  unicodeRanges: Range[];     // for the glyph map
  installedByApp?: boolean;   // a system font skrifttype installed for all users
}

/** Outcome of converting one face; err is non-empty when that face failed. */
export interface ConvertResult { faceIndex: number; path: string; err: string; }

/** Outcome of an "install/uninstall for everyone" action. */
export interface InstallResult { succeeded: string[]; failed: { file: string; error: string }[]; }

export type ActivationSourceKind = 'manual' | 'collection';
/** A font is active iff it has >=1 source key. Keys: "manual" | "collection:<id>". */
export interface ActivationState { sources: Record<number, string[]>; }
export interface ActivationDiff {
  loaded: number[]; unloaded: number[]; noop: number[];
  failed?: { fontId: number; error: string }[];   // fonts GDI refused or that can't be toggled
}

export interface Collection { id: number; name: string; color: string; fontIds: number[]; active: boolean; }
export interface NewCollection { name: string; color: string; fontIds?: number[]; }

export interface OpenTypeFeature { tag: string; name: string; kind: 'gsub' | 'gpos'; }

export interface WatchedDir { id: number; path: string; recursive: boolean; enabled: boolean; }
export interface Settings {
  theme: 'light' | 'dark' | 'system';
  defaultSampleText: string;
  previewSize: number;
  watchedDirs: WatchedDir[];
}

export interface FontQuery {
  search?: string;
  classifications?: FilterClass[];
  scope?: 'all' | 'system' | 'active' | number; // number = collectionId
  sort?: 'family' | 'styles';
}

export type ReportType = 'specimen' | 'overview';
export interface ReportRequest { type: ReportType; scope: 'all' | 'system' | 'active' | number; sampleText: string; }
export interface ReportData { title: string; type: ReportType; sampleText: string; fonts: FontRecord[]; }

/** Result of a backend rescan (library:changed event). */
export interface ScanReport { scanned: number; skipped: number; errored: number; missing: number; error?: string; }

export interface ActivationProgress { done: number; total: number }

export interface LibraryEventHandlers {
  scanning?: (running: boolean) => void;
  changed?: (report: ScanReport) => void;
}
