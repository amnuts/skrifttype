import { describe, it, expect, vi, beforeEach } from 'vitest';

// Stand-ins for the generated Go bindings.
const LibraryService = { ListFonts: vi.fn(), GetFont: vi.fn(), GetGlyphRanges: vi.fn(), Rescan: vi.fn(), IsScanning: vi.fn() };
const ActivationService = {
  GetActiveSet: vi.fn(), ActivateManual: vi.fn(), DeactivateManual: vi.fn(),
  ActivateCollection: vi.fn(), DeactivateCollection: vi.fn(), Rebroadcast: vi.fn(),
};
const CollectionService = {
  ListCollections: vi.fn(), CreateCollection: vi.fn(), AddToCollection: vi.fn(),
  RemoveFromCollection: vi.fn(), RenameCollection: vi.fn(), DeleteCollection: vi.fn(),
};
const SettingsService = {
  ListWatchedRoots: vi.fn(), AddWatchedRoot: vi.fn(), RemoveWatchedRoot: vi.fn(),
  SetWatchedRootEnabled: vi.fn(), PickFolder: vi.fn(), GetPreference: vi.fn(), SetPreference: vi.fn(),
};
const ConvertService = { ConvertFont: vi.fn(), GetConvertFolder: vi.fn(), SetConvertFolder: vi.fn(), InWatchedRoot: vi.fn() };
vi.mock('../../bindings/github.com/amnuts/skrifttype/internal/api', () => ({
  LibraryService, ActivationService, CollectionService, SettingsService, ConvertService,
}));
vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) } }));

const { WailsDataSource } = await import('./wails-data-source');

const dto = {
  id: 7, family: 'Inter', cssFamily: 'skrift-7', classification: 'sans', source: 'library',
  styles: [{ id: 7, styleName: 'Medium', weight: 500, italic: false, path: 'D:\\f\\a.ttf', url: '/fontfile/7', available: true }],
  styleCount: 1, designer: null, foundry: null, version: null, format: 'ttf', available: true,
  parseError: null, isVariable: false, gsubFeatures: ['ss01'], gposFeatures: null, unicodeRanges: null,
};

describe('WailsDataSource', () => {
  let ds: InstanceType<typeof WailsDataSource>;
  beforeEach(() => { vi.clearAllMocks(); ds = new WailsDataSource(); });

  it('splits a numeric scope into collection + id and normalises null slices', async () => {
    LibraryService.ListFonts.mockResolvedValue([dto]);
    const fonts = await ds.listFonts({ search: 'in', scope: 3 });
    expect(LibraryService.ListFonts).toHaveBeenCalledWith({
      search: 'in', classifications: [], scope: 'collection', collectionId: 3, sort: 'family',
    });
    expect(fonts[0].gposFeatures).toEqual([]);
    expect(fonts[0].unicodeRanges).toEqual([]);
    expect(fonts[0].styles[0].url).toBe('/fontfile/7');
  });

  it('maps activatable from the DTO', async () => {
    const style = dto.styles[0];
    LibraryService.ListFonts.mockResolvedValue([
      { ...dto, styles: [{ ...style, activatable: false }] },
      { ...dto, styles: [{ ...style, activatable: true }] },
      { ...dto, styles: [style] },
    ]);
    const fonts = await ds.listFonts({});
    expect(fonts[0].styles[0].activatable).toBe(false);
    expect(fonts[1].styles[0].activatable).toBe(true);
    expect(fonts[2].styles[0].activatable).toBeUndefined();
  });

  it('treats a null list as empty', async () => {
    LibraryService.ListFonts.mockResolvedValue(null);
    expect(await ds.listFonts({})).toEqual([]);
  });

  it('parses active-set keys back to numbers and drops empty entries', async () => {
    ActivationService.GetActiveSet.mockResolvedValue({ '7': ['manual', 'collection:2'], '9': null });
    expect(await ds.getActiveSet()).toEqual({ sources: { 7: ['manual', 'collection:2'] } });
  });

  it('derives feature names from the record', async () => {
    LibraryService.GetFont.mockResolvedValue(dto);
    expect(await ds.getFeatures(7)).toEqual([{ tag: 'ss01', name: 'Stylistic Set 1', kind: 'gsub' }]);
  });

  it('maps empty preference to null', async () => {
    SettingsService.GetPreference.mockResolvedValue('');
    expect(await ds.getPreference('x')).toBeNull();
  });

  it('converts with the saved folder when outDir is empty and keeps per-face errors', async () => {
    const out = [{ faceIndex: 0, path: 'D:/c/a.ttf', err: '' }, { faceIndex: 1, path: '', err: 'exists' }];
    ConvertService.ConvertFont.mockResolvedValue(out);
    expect(await ds.convertFont(7, true, '', false)).toEqual(out);
    expect(ConvertService.ConvertFont).toHaveBeenCalledWith(7, true, '', false);
  });

  it('treats a null convert result as empty', async () => {
    ConvertService.ConvertFont.mockResolvedValue(null);
    expect(await ds.convertFont(7, false, 'D:/c', true)).toEqual([]);
    expect(ConvertService.ConvertFont).toHaveBeenCalledWith(7, false, 'D:/c', true);
  });

  it('delegates the conversion folder methods', async () => {
    ConvertService.GetConvertFolder.mockResolvedValue('D:/c');
    ConvertService.InWatchedRoot.mockResolvedValue(true);
    expect(await ds.getConvertFolder()).toBe('D:/c');
    await ds.setConvertFolder('');
    expect(ConvertService.SetConvertFolder).toHaveBeenCalledWith('');
    expect(await ds.inWatchedRoot('D:/c')).toBe(true);
    expect(ConvertService.InWatchedRoot).toHaveBeenCalledWith('D:/c');
  });
});
