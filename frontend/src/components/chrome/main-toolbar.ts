import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { libraryContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import type { FilterClass } from '../../data/types';
import { StoreConnector } from '../../state/store-connector';
import './pane-tabs';
import './phrase-picker';
import type { Pane } from './pane-tabs';
import '../shared/ui-icon';
import '../shared/ui-chip';

const CLASSES: FilterClass[] = ['serif', 'sans', 'mono', 'script', 'display'];

@customElement('main-toolbar')
export class MainToolbar extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  @property() pane: Pane = 'preview';
  readonly connector = new StoreConnector(this, () => [this.lib]);
  static styles = css`
    .bar{display:flex;align-items:center;gap:var(--sp-3);padding:var(--sp-3) var(--sp-4);
      background:var(--panel);border-bottom:1px solid var(--border)}
    .search{flex:0 0 280px;display:flex;align-items:center;gap:var(--sp-2);height:32px;padding:0 var(--sp-3);
      background:var(--panel-2);border:1px solid var(--border);border-radius:var(--r-md);color:var(--muted)}
    .search input{border:none;background:none;outline:none;width:100%;color:var(--text);font-size:var(--fs-sm)}
    .chips{display:flex;gap:var(--sp-2)} .spacer{flex:1}
  `;
  render() {
    return html`<div class="bar">
      <label class="search"><ui-icon name="search" size="16"></ui-icon>
        <input placeholder="Search families…" .value=${this.lib.search}
          @input=${(e: Event) => this.lib.setSearch((e.target as HTMLInputElement).value)} /></label>
      <div class="chips">${CLASSES.map((c) => html`<ui-chip ?on=${this.lib.classifications.includes(c)}
        @toggle=${() => this.lib.toggleClass(c)}>${c[0].toUpperCase() + c.slice(1)}</ui-chip>`)}</div>
      <span class="spacer"></span>
      <phrase-picker></phrase-picker>
      <pane-tabs .active=${this.pane} @select=${(e: CustomEvent<Pane>) =>
        this.dispatchEvent(new CustomEvent('select-pane', { detail: e.detail, bubbles: true, composed: true }))}></pane-tabs>
    </div>`;
  }
}
