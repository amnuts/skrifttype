import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { libraryContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import { StoreConnector } from '../../state/store-connector';
import '../shared/ui-slider';
import './comparison-cell';

@customElement('comparison-pane')
export class ComparisonPane extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  readonly connector = new StoreConnector(this, () => [this.lib]);
  @state() text = 'The quick brown fox jumps over the lazy dog'; @state() size = 40;
  static styles = css`
    :host{display:flex;flex-direction:column;height:100%}
    .controls{padding:var(--sp-4);border-bottom:1px solid var(--border);display:grid;gap:var(--sp-3)}
    input{height:32px;padding:0 var(--sp-3);border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel-2);color:var(--text)}
    .grid{flex:1;overflow:auto;padding:var(--sp-4);display:grid;gap:var(--sp-3);
      grid-template-columns:repeat(auto-fit,minmax(220px,1fr))}
    .empty{margin:auto;color:var(--muted);text-align:center;padding:var(--sp-7)}
  `;
  render() {
    const fonts = this.lib.fonts.filter((f) => this.lib.selectedIds.includes(f.id));
    if (fonts.length < 2) return html`<div class="empty">Ctrl/Shift-click 2–4 fonts in the list to compare them side by side.</div>`;
    return html`
      <div class="controls">
        <input .value=${this.text} @input=${(e: Event) => (this.text = (e.target as HTMLInputElement).value)} />
        <ui-slider label="Size" unit="px" min="12" max="120" .value=${this.size} @change=${(e: CustomEvent<number>) => (this.size = e.detail)}></ui-slider>
      </div>
      <div class="grid">${fonts.map((f) => html`<comparison-cell .font=${f} .text=${this.text} .size=${this.size}></comparison-cell>`)}</div>`;
  }
}
