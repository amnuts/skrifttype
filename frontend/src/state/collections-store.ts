import { dataSource } from '../data';
import type { Collection, NewCollection } from '../data/types';

type Listener = () => void;

export class CollectionsStore {
  collections: Collection[] = [];
  private listeners = new Set<Listener>();
  subscribe(fn: Listener) { this.listeners.add(fn); return () => this.listeners.delete(fn); }
  private emit() { this.listeners.forEach((l) => l()); }

  async refresh() { this.collections = await dataSource.listCollections(); this.emit(); }
  async create(input: NewCollection) { await dataSource.createCollection(input); await this.refresh(); }
  async addFonts(cid: number, ids: number[]) { await dataSource.addToCollection(cid, ids); await this.refresh(); }
  async removeFonts(cid: number, ids: number[]) { await dataSource.removeFromCollection(cid, ids); await this.refresh(); }
  async rename(cid: number, name: string) { await dataSource.renameCollection(cid, name); await this.refresh(); }
  async update(c: Collection, patch: { name: string; color: string }) {
    if (patch.name !== c.name) await dataSource.renameCollection(c.id, patch.name);
    if (patch.color !== c.color) await dataSource.setCollectionColor(c.id, patch.color);
    await this.refresh();
  }
  async remove(cid: number) { await dataSource.deleteCollection(cid); await this.refresh(); }
}
