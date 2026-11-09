import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { OpenTypeFeature } from '../../data/types';

@customElement('feature-toggles')
export class FeatureToggles extends LitElement {
  @property({ attribute: false }) features: OpenTypeFeature[] = [];
  @property({ attribute: false }) active = new Set<string>();
  static styles = css`
    .wrap{display:flex;flex-wrap:wrap;gap:var(--sp-2)}
    button{font-size:var(--fs-xs);padding:4px 9px;border-radius:var(--r-sm);border:1px solid var(--border);
      background:var(--panel-2);color:var(--muted);cursor:pointer}
    button.on{background:var(--accent);color:var(--accent-text);border-color:transparent}
    .lbl{font-size:var(--fs-xs);color:var(--muted);text-transform:uppercase;letter-spacing:.1em;margin-bottom:var(--sp-2)}
  `;
  render() {
    if (this.features.length === 0) return html``;
    return html`<div class="lbl">OpenType features</div><div class="wrap">
      ${this.features.map((f) => html`<button class=${this.active.has(f.tag) ? 'on' : ''} title=${f.name}
        @click=${() => this.dispatchEvent(new CustomEvent('toggle-feature', { detail: f.tag, bubbles: true, composed: true }))}>${f.tag}</button>`)}
    </div>`;
  }
}
