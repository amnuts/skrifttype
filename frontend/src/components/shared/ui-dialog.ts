import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import './ui-icon';

@customElement('ui-dialog')
export class UiDialog extends LitElement {
  @property({ type: Boolean, reflect: true }) open = false;
  @property() heading = '';
  static styles = css`
    *{box-sizing:border-box}
    :host{display:none} :host([open]){display:block}
    .scrim{position:fixed;inset:0;background:rgba(10,15,25,.45);display:flex;align-items:center;justify-content:center;z-index:50}
    .card{background:var(--panel);border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);
      width:min(560px,92vw);max-height:86vh;display:flex;flex-direction:column;overflow:hidden}
    header{flex:none;display:flex;align-items:center;justify-content:space-between;padding:var(--sp-4) var(--sp-5);border-bottom:1px solid var(--border)}
    h2{margin:0;font-size:var(--fs-lg)}
    .body{flex:1 1 auto;min-height:0;overflow-y:auto;overflow-x:hidden;padding:var(--sp-5)}
    button.x{background:none;border:none;color:var(--muted);cursor:pointer}
  `;
  private close() { this.dispatchEvent(new CustomEvent('close', { bubbles: true, composed: true })); }
  render() {
    return html`<div class="scrim" @click=${(e: Event) => e.target === e.currentTarget && this.close()}>
      <div class="card" role="dialog" aria-modal="true">
        <header><h2>${this.heading}</h2><button class="x" @click=${this.close}><ui-icon name="close"></ui-icon></button></header>
        <div class="body"><slot></slot></div>
      </div></div>`;
  }
}
