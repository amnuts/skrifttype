import type { ReactiveController, ReactiveControllerHost } from 'lit';

interface Subscribable { subscribe(fn: () => void): () => void; }

/**
 * Bridges the plain `subscribe()/emit()` stores to Lit's update lifecycle.
 *
 * `@consume` only re-renders a component when the *provided value identity*
 * changes. Our stores are stable singletons that mutate internally, so consumers
 * would never re-render on store changes. This controller lazily subscribes the
 * host to each store (once it has been resolved from context) and calls
 * `host.requestUpdate()` whenever a store emits.
 */
export class StoreConnector implements ReactiveController {
  private subs = new Map<Subscribable, () => void>();

  constructor(
    private host: ReactiveControllerHost,
    private stores: () => Array<Subscribable | undefined>,
  ) {
    host.addController(this);
  }

  // Runs before each render; by then `@consume` properties are populated.
  hostUpdate() {
    for (const store of this.stores()) {
      if (store && !this.subs.has(store)) {
        this.subs.set(store, store.subscribe(() => this.host.requestUpdate()));
      }
    }
  }

  hostDisconnected() {
    this.subs.forEach((unsub) => unsub());
    this.subs.clear();
  }
}
