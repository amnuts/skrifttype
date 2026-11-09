import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('ui-button')
export class UiButton extends LitElement {
  @property() variant: 'solid' | 'ghost' | 'subtle' = 'subtle';
  @property({ type: Boolean }) active = false;
  @property({ type: Boolean, reflect: true }) disabled = false;
  static styles = css`
    button{display:inline-flex;align-items:center;gap:var(--sp-2);border:1px solid transparent;
      border-radius:var(--r-md);padding:var(--sp-2) var(--sp-3);font-size:var(--fs-sm);font-weight:600;
      cursor:pointer;background:transparent;color:var(--text);transition:background var(--dur) var(--ease),transform var(--dur) var(--ease)}
    button:active{transform:translateY(1px)}
    :host([variant="solid"]) button{background:var(--accent);color:var(--accent-text)}
    :host([variant="subtle"]) button{background:var(--panel-2);border-color:var(--border)}
    :host([variant="ghost"]) button{color:var(--muted)}
    :host([variant="ghost"]) button:hover{background:var(--panel-2);color:var(--text)}
    :host([active]) button{background:var(--accent);color:var(--accent-text)}
    :host([disabled]){pointer-events:none;opacity:.5}
  `;
  render() { return html`<button part="btn" ?disabled=${this.disabled}><slot></slot></button>`; }
}
