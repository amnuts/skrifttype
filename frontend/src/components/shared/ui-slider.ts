import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';

@customElement('ui-slider')
export class UiSlider extends LitElement {
  @property({ type: Number }) value = 50;
  @property({ type: Number }) min = 0;
  @property({ type: Number }) max = 100;
  @property() label = '';
  @property() unit = '';
  static styles = css`
    .row{display:flex;justify-content:space-between;font-size:var(--fs-xs);color:var(--muted);margin-bottom:var(--sp-1)}
    input{width:100%;user-select:none;-webkit-user-drag:none;-webkit-appearance:none;height:4px;border-radius:2px;background:var(--border);outline:none}
    input::-webkit-slider-thumb{-webkit-appearance:none;width:14px;height:14px;border-radius:50%;background:var(--accent);cursor:pointer;box-shadow:var(--shadow-sm)}
  `;
  private onInput(e: Event) {
    this.value = (e.target as HTMLInputElement).valueAsNumber;
    this.dispatchEvent(new CustomEvent('change', { detail: this.value, bubbles: true, composed: true }));
  }
  render() {
    return html`
      ${this.label ? html`<div class="row"><span>${this.label}</span><span>${this.value}${this.unit}</span></div>` : ''}
      <input type="range" min=${this.min} max=${this.max} .value=${String(this.value)} @input=${this.onInput} @dragstart=${(e: Event) => e.preventDefault()} />`;
  }
}
