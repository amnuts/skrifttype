import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('status-bar')
export class StatusBar extends LitElement {
  @property({ type: Boolean }) loading = false;
  /** Backend is rescanning watched folders / system fonts. */
  @property({ type: Boolean }) scanning = false;
  @property({ type: Number }) selected = 0;
  static styles = css`
    .bar{height:30px;display:flex;align-items:center;gap:var(--sp-3);padding:0 var(--sp-4);
      background:var(--panel-2);border-top:1px solid var(--border);font-size:var(--fs-xs);color:var(--muted)}
    .spacer{flex:1} .dot{width:7px;height:7px;border-radius:50%;background:var(--accent);animation:p 1s infinite}
    @keyframes p{50%{opacity:.3}}
  `;
  render() {
    return html`<div class="bar">
      ${this.scanning ? html`<span class="dot"></span> Scanning folders…`
        : this.loading ? html`<span class="dot"></span> Loading…` : html`Ready`}
      <span class="spacer"></span>${this.selected ? `${this.selected} selected` : ''}</div>`;
  }
}
