import { LitElement, html, css } from 'lit';
import { customElement } from 'lit/decorators.js';
import { consume } from '@lit/context';
import '@lit-labs/virtualizer';
import { libraryContext, activationContext, collectionsContext, preferencesContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import type { ActivationStore } from '../../state/activation-store';
import type { CollectionsStore } from '../../state/collections-store';
import type { PreferencesStore } from '../../state/preferences-store';
import { setDraggedFonts } from '../../state/font-drag';
import type { FontRecord } from '../../data/types';
import { StoreConnector } from '../../state/store-connector';
import './font-row';
import '../shared/ui-slider';
import { LIST_SIZE_MAX, LIST_SIZE_MIN } from '../../state/preferences-store';

@customElement('library-pane')
export class LibraryPane extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  @consume({ context: activationContext, subscribe: true }) act!: ActivationStore;
  @consume({ context: collectionsContext, subscribe: true }) cols!: CollectionsStore;
  @consume({ context: preferencesContext, subscribe: true }) prefs!: PreferencesStore;
  readonly connector = new StoreConnector(this, () => [this.lib, this.act, this.cols]);
  // Only re-render for the prefs this list shows. Other prefs (the preview
  // size slider) emit on every drag step, and rebuilding the whole virtualised
  // list for each would make that slider stutter.
  private offPrefs?: () => void;
  private shown = '';
  private watchPrefs() {
    if (this.offPrefs || !this.prefs) return;
    this.shown = `${this.prefs.sampleText}|${this.prefs.listSize}`;
    this.offPrefs = this.prefs.subscribe(() => {
      const key = `${this.prefs.sampleText}|${this.prefs.listSize}`;
      if (key !== this.shown) { this.shown = key; this.requestUpdate(); }
    });
  }
  willUpdate() { this.watchPrefs(); }
  disconnectedCallback() { super.disconnectedCallback(); this.offPrefs?.(); this.offPrefs = undefined; }
  static styles = css`
    :host{display:flex;flex-direction:column;height:100%;background:var(--panel)}
    .size{flex:none;padding:var(--sp-2) var(--sp-4);border-bottom:1px solid var(--border)}
    lit-virtualizer{flex:1;min-height:0;display:block} .empty{padding:var(--sp-7);text-align:center;color:var(--muted)}
  `;
  private onToggle(e: CustomEvent<number>) { this.act.toggleManual(e.detail); }
  /** Dragging a selected row drags the whole selection. */
  private onDrag(e: CustomEvent<{ id: number; dataTransfer: DataTransfer }>) {
    const sel = this.lib.selectedIds;
    if (sel.length > 1 && sel.includes(e.detail.id)) setDraggedFonts(e.detail.dataTransfer, sel);
  }
  private async onRemove(e: CustomEvent<number>) {
    const scope = this.lib.scope;
    if (typeof scope !== 'number') return;
    const sel = this.lib.selectedIds;
    const ids = sel.length > 1 && sel.includes(e.detail) ? sel : [e.detail];
    await this.cols.removeFonts(scope, ids);
    await Promise.all([this.lib.refresh(), this.act.refresh()]);
  }
  private onSelect(e: CustomEvent<{ id: number; multi: boolean }>) {
    e.detail.multi ? this.lib.toggleMulti(e.detail.id) : this.lib.selectSingle(e.detail.id);
  }
  render() {
    if (!this.lib.loading && this.lib.fonts.length === 0) {
      const col = typeof this.lib.scope === 'number' ? this.cols.collections.find((c) => c.id === this.lib.scope) : undefined;
      const filtered = this.lib.search.trim() !== '' || this.lib.classifications.length > 0;
      return html`<div class="empty">${col && col.fontIds.length === 0
        ? 'This collection is empty — drag fonts from Available fonts onto it in the sidebar.'
        : col && filtered ? 'No fonts in this collection match your search or filters.'
        : this.lib.scope === 'active' ? (filtered ? 'No active fonts match your search or filters.' : "You don't have any fonts currently activated.")
        : 'No fonts match your filters.'}</div>`;
    }
    const inCollection = typeof this.lib.scope === 'number';
    return html`<div class="size"><ui-slider label="Text size" unit="px" min=${LIST_SIZE_MIN} max=${LIST_SIZE_MAX} .value=${this.prefs.listSize}
        @change=${(e: CustomEvent<number>) => this.prefs.setListSize(e.detail)}></ui-slider></div>
      <lit-virtualizer scroller
      @toggle-active=${this.onToggle} @select-font=${this.onSelect}
      @drag-font=${this.onDrag} @remove-from-collection=${this.onRemove}
      .items=${this.lib.fonts}
      .renderItem=${(f: FontRecord) => html`<font-row .font=${f} .sample=${this.prefs.sampleText} .size=${this.prefs.listSize} .active=${this.act.isActive(f.id)}
        .selected=${this.lib.selectedIds.includes(f.id)} ?removable=${inCollection}></font-row>`}>
    </lit-virtualizer>`;
  }
}
