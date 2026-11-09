import type { Collection } from '../data/types';

// fontIds are filled at runtime by MockDataSource (it picks members by classification).
export const SEED_COLLECTIONS: Omit<Collection, 'fontIds'>[] = [
  { id: 1, name: 'Web UI', color: 'var(--c2)', active: true },
  { id: 2, name: 'Print & Editorial', color: 'var(--c3)', active: false },
  { id: 3, name: 'Branding', color: 'var(--c5)', active: false },
];
