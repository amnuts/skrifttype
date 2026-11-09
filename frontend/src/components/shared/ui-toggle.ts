import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('ui-toggle')
export class UiToggle extends LitElement {
  @property({ type: Boolean, reflect: true }) on = false;
  @property({ type: Boolean }) disabled = false;
  static styles = css`
    button{width:34px;height:19px;border-radius:var(--r-pill);background:var(--border);border:none;
      position:relative;cursor:pointer;padding:0;transition:background var(--dur) var(--ease)}
    button[disabled]{opacity:.4;cursor:not-allowed}
    span{position:absolute;top:2px;left:2px;width:15px;height:15px;border-radius:50%;background:#fff;
      box-shadow:0 1px 2px rgba(0,0,0,.35);transition:left var(--dur) var(--ease)}
    :host([on]) button{background:var(--accent)} :host([on]) span{left:17px}
  `;
  private toggle() { if (this.disabled) return; this.dispatchEvent(new CustomEvent('toggle', { bubbles: true, composed: true })); }
  render() { return html`<button ?disabled=${this.disabled} role="switch" aria-checked=${this.on} @click=${this.toggle}><span></span></button>`; }
}
