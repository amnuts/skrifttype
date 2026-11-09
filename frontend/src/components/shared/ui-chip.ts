import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('ui-chip')
export class UiChip extends LitElement {
  @property({ type: Boolean, reflect: true }) on = false;
  static styles = css`
    button{font-size:var(--fs-sm);padding:5px 11px;border-radius:var(--r-pill);border:1px solid var(--border);
      background:var(--panel-2);color:var(--muted);cursor:pointer;transition:all var(--dur) var(--ease)}
    :host([on]) button{background:var(--accent);color:var(--accent-text);border-color:transparent}
  `;
  render() { return html`<button @click=${() => this.dispatchEvent(new CustomEvent('toggle', { bubbles: true, composed: true }))}><slot></slot></button>`; }
}
