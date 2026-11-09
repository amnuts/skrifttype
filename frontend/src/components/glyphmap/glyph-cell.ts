import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { codepointHex } from '../../services/format';

@customElement('glyph-cell')
export class GlyphCell extends LitElement {
  @property({ type: Number }) cp = 0;
  @property() fontFamily = '';
  @property({ type: Number }) size = 32;
  static styles = css`
    button{width:100%;aspect-ratio:1;border:1px solid var(--border);border-radius:var(--r-sm);background:var(--panel);
      cursor:pointer;display:flex;align-items:center;justify-content:center;color:var(--text);transition:all var(--dur) var(--ease)}
    button:hover{border-color:var(--accent);background:var(--accent-weak)}
  `;
  private copy() { this.dispatchEvent(new CustomEvent('copy-glyph', { detail: { cp: this.cp, char: String.fromCodePoint(this.cp) }, bubbles: true, composed: true })); }
  render() {
    return html`<button title=${codepointHex(this.cp)} style="font-family:${this.fontFamily};font-size:${this.size}px" @click=${this.copy}>${String.fromCodePoint(this.cp)}</button>`;
  }
}
