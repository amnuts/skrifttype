import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { Collection } from '../../data/types';
import '../shared/ui-dialog';
import '../shared/ui-button';

const COLORS = Array.from({ length: 16 }, (_, i) => `var(--c${i + 1})`);

@customElement('collection-dialog')
export class CollectionDialog extends LitElement {
  @property({ type: Boolean, reflect: true }) open = false;
  /** Number of fonts currently selected in the library; offered as initial members. */
  @property({ type: Number }) selectedCount = 0;
  /** When set, the dialog edits this collection instead of creating one. */
  @property({ attribute: false }) collection: Collection | null = null;
  @state() name = ''; @state() color = COLORS[0]; @state() includeSelected = true;
  static styles = css`
    .field{margin-bottom:var(--sp-4)} label{display:block;font-size:var(--fs-sm);color:var(--muted);margin-bottom:var(--sp-2)}
    input{box-sizing:border-box;width:100%;height:34px;padding:0 var(--sp-3);border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel-2);color:var(--text)}
    .swatches{display:flex;flex-wrap:wrap;gap:var(--sp-2)} .sw{width:24px;height:24px;border-radius:var(--r-sm);cursor:pointer;border:2px solid transparent}
    .sw.on{border-color:var(--text)} .actions{display:flex;justify-content:flex-end;gap:var(--sp-2);margin-top:var(--sp-5)}
    .check{display:flex;align-items:center;gap:var(--sp-2);font-size:var(--fs-sm);color:var(--text)} .check input{width:auto;height:auto}
  `;
  protected willUpdate(changed: Map<string, unknown>) {
    if ((changed.has('open') || changed.has('collection')) && this.open) {
      this.name = this.collection?.name ?? '';
      this.color = this.collection?.color ?? COLORS[0];
    }
  }
  private submit() {
    if (!this.name.trim()) return;
    if (this.collection) {
      this.dispatchEvent(new CustomEvent('save', { detail: { name: this.name.trim(), color: this.color }, bubbles: true, composed: true }));
      return;
    }
    const includeSelected = this.includeSelected && this.selectedCount > 0;
    this.dispatchEvent(new CustomEvent('create', { detail: { name: this.name.trim(), color: this.color, includeSelected }, bubbles: true, composed: true }));
    this.name = '';
  }
  render() {
    return html`<ui-dialog ?open=${this.open} heading=${this.collection ? 'Edit collection' : 'New collection'} @close=${() => this.dispatchEvent(new CustomEvent('close', { bubbles: true, composed: true }))}>
      <div class="field"><label>Name</label><input .value=${this.name} @input=${(e: Event) => (this.name = (e.target as HTMLInputElement).value)} placeholder="e.g. Brand 2026" @keydown=${(e: KeyboardEvent) => e.key === 'Enter' && this.submit()} /></div>
      <div class="field"><label>Colour</label><div class="swatches">${COLORS.map((c) => html`<div class="sw ${c === this.color ? 'on' : ''}" style="background:${c}" @click=${() => (this.color = c)}></div>`)}</div></div>
      ${!this.collection && this.selectedCount > 0 ? html`<label class="check"><input type="checkbox" .checked=${this.includeSelected}
        @change=${(e: Event) => (this.includeSelected = (e.target as HTMLInputElement).checked)} />
        Add the ${this.selectedCount} selected font${this.selectedCount === 1 ? '' : 's'}</label>` : ''}
      <div class="actions"><ui-button variant="ghost" @click=${() => this.dispatchEvent(new CustomEvent('close', { bubbles: true, composed: true }))}>Cancel</ui-button><ui-button variant="solid" @click=${this.submit}>${this.collection ? 'Save' : 'Create'}</ui-button></div>
    </ui-dialog>`;
  }
}
