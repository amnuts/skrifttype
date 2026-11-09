import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import '../shared/ui-slider';

@customElement('preview-controls')
export class PreviewControls extends LitElement {
  @property({ type: Number }) size = 72;
  @property({ type: Number }) leading = 110;
  @property({ type: Number }) tracking = 0;
  static styles = css`:host{display:grid;gap:var(--sp-3)}`;
  private fire(name: string, v: number) { this.dispatchEvent(new CustomEvent(name, { detail: v, bubbles: true, composed: true })); }
  render() {
    return html`
      <ui-slider label="Size" unit="px" min="8" max="200" .value=${this.size} @change=${(e: CustomEvent<number>) => this.fire('size', e.detail)}></ui-slider>
      <ui-slider label="Line height" unit="%" min="80" max="200" .value=${this.leading} @change=${(e: CustomEvent<number>) => this.fire('leading', e.detail)}></ui-slider>
      <ui-slider label="Tracking" unit="em/1000" min="-50" max="200" .value=${this.tracking} @change=${(e: CustomEvent<number>) => this.fire('tracking', e.detail)}></ui-slider>`;
  }
}
