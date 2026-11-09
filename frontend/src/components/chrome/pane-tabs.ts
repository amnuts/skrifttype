import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

export type Pane = 'preview' | 'glyphs' | 'compare' | 'report';
const TABS: Pane[] = ['preview', 'glyphs', 'compare', 'report'];

@customElement('pane-tabs')
export class PaneTabs extends LitElement {
  @property() active: Pane = 'preview';
  static styles = css`
    .tabs{display:flex;gap:2px;background:var(--panel-2);padding:3px;border-radius:var(--r-md);border:1px solid var(--border)}
    button{font-size:var(--fs-sm);text-transform:capitalize;padding:5px 12px;border:none;border-radius:var(--r-sm);
      background:transparent;color:var(--muted);cursor:pointer;transition:all var(--dur) var(--ease)}
    button.on{background:var(--accent);color:var(--accent-text)}
  `;
  private pick(p: Pane) { this.dispatchEvent(new CustomEvent('select', { detail: p, bubbles: true, composed: true })); }
  render() {
    return html`<div class="tabs">${TABS.map((t) => html`<button class=${t === this.active ? 'on' : ''} @click=${() => this.pick(t)}>${t}</button>`)}</div>`;
  }
}
