import { describe, it, expect, beforeEach } from 'vitest';
import { MockDataSource } from './mock-data-source';

describe('MockDataSource', () => {
  it('lists fonts and filters by search', async () => {
    const ds = new MockDataSource();
    const all = await ds.listFonts({});
    expect(all.length).toBeGreaterThan(0);
    const arial = await ds.listFonts({ search: 'arial' });
    expect(arial.every((f) => f.family.toLowerCase().includes('arial'))).toBe(true);
  });

  it('refcounts activation: manual + collection sources are additive', async () => {
    const ds = new MockDataSource();
    const cols = await ds.listCollections();

    // Use an initially-inactive collection so we can observe the 0→1→2→1→0 transition.
    const col = cols.find((c) => !c.active)!;
    expect(col).toBeDefined();

    // Find a member that is not already active (not seeded via another collection or manual).
    const initialState = await ds.getActiveSet();
    const member = col.fontIds.find((id) => initialState.sources[id] === undefined)!;
    expect(member).toBeDefined();

    // Before anything: member has no sources.
    expect(initialState.sources[member]).toBeUndefined();

    // Step 1: activate the collection -> member gets exactly one source key.
    await ds.activateCollection(col.id);
    let state = await ds.getActiveSet();
    expect(state.sources[member]).toEqual([`collection:${col.id}`]);

    // Step 2: manually activate the same font -> now two sources (additive refcount).
    await ds.activateManual(member);
    state = await ds.getActiveSet();
    expect(state.sources[member]).toHaveLength(2);
    expect(state.sources[member]).toContain(`collection:${col.id}`);
    expect(state.sources[member]).toContain('manual');

    // Step 3: deactivate the collection -> font still active via manual only.
    await ds.deactivateCollection(col.id);
    state = await ds.getActiveSet();
    expect(state.sources[member]).toEqual(['manual']);

    // Step 4: remove manual -> font is now unloaded entirely.
    const diff = await ds.deactivateManual(member);
    state = await ds.getActiveSet();
    expect(state.sources[member]).toBeUndefined();
    expect(diff.unloaded).toContain(member);
  });
});

describe('MockDataSource watched roots + preferences', () => {
  beforeEach(() => localStorage.clear());

  it('starts with no watched folders (no examples)', async () => {
    const ds = new MockDataSource();
    expect(await ds.listWatchedRoots()).toEqual([]);
  });

  it('adds, disables, and removes a watched root', async () => {
    const ds = new MockDataSource();
    const r = await ds.addWatchedRoot('C:\\Fonts', true);
    expect(r.id).toBeGreaterThan(0);
    await ds.setWatchedRootEnabled(r.id, false);
    expect((await ds.listWatchedRoots())[0].enabled).toBe(false);
    await ds.removeWatchedRoot(r.id);
    expect(await ds.listWatchedRoots()).toEqual([]);
  });

  it('round-trips a preference', async () => {
    const ds = new MockDataSource();
    expect(await ds.getPreference('onboarding_seen')).toBeNull();
    await ds.setPreference('onboarding_seen', '1');
    expect(await ds.getPreference('onboarding_seen')).toBe('1');
  });
});
