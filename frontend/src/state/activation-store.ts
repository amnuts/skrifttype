import { dataSource } from '../data';

type Listener = () => void;

export class ActivationStore {
  private sources: Record<number, string[]> = {};
  private listeners = new Set<Listener>();

  subscribe(fn: Listener) { this.listeners.add(fn); return () => this.listeners.delete(fn); }
  private emit() { this.listeners.forEach((l) => l()); }

  /** Progress of a running bulk on/off, or null. */
  bulk: { done: number; total: number } | null = null;

  async refresh() { this.sources = (await dataSource.getActiveSet()).sources; this.emit(); }

  isActive(id: number) { return (this.sources[id]?.length ?? 0) > 0; }
  get activeCount() { return Object.values(this.sources).filter((s) => s.length > 0).length; }

  /**
   * Optimistic on/off toggle with rollback on error. Turning a font off drops
   * every source holding it, including an active collection; switching the
   * collection off and on again brings it back.
   */
  async toggleManual(id: number) {
    const had = this.isActive(id);
    const prev = structuredClone(this.sources);
    // optimistic
    if (had) delete this.sources[id];
    else this.sources[id] = ['manual'];
    this.emit();
    try {
      if (had) await dataSource.deactivate(id);
      else await dataSource.activateManual(id);
      await this.refresh();
    } catch { this.sources = prev; this.emit(); }
  }

  /** Switches every activatable library font on or off. */
  async setAllActive(active: boolean) {
    // The backend applies the change in chunks and reports after each, so the
    // list can follow along instead of jumping at the end.
    let pending = false;
    const off = dataSource.onActivationProgress((p) => {
      this.bulk = p;
      this.emit();
      if (pending) return;
      pending = true;
      void this.refresh().finally(() => { pending = false; });
    });
    this.bulk = { done: 0, total: 0 };
    try {
      if (active) await dataSource.activateAllAvailable(); else await dataSource.deactivateAll();
    } finally {
      off();
      this.bulk = null;
      await this.refresh();
    }
  }

  async setCollectionActive(cid: number, active: boolean) {
    if (active) await dataSource.activateCollection(cid); else await dataSource.deactivateCollection(cid);
    await this.refresh();
  }
}
