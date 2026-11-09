import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { FontRecord } from '../../data/types';
import { cssFamilyFor } from '../../services/font-face';
import { featureSettings } from '../../services/format';

@customElement('comparison-cell')
export class ComparisonCell extends LitElement {
  @property({ attribute: false }) font!: FontRecord;
  @property() text = '';
  @property({ type: Number }) size = 48;
  @property({ attribute: false }) features = new Set<string>();
  static styles = css`
    :host{display:flex;flex-direction:column;border:1px solid var(--border);border-radius:var(--r-md);overflow:hidden;background:var(--panel)}
    header{padding:var(--sp-2) var(--sp-3);border-bottom:1px solid var(--border);font-size:var(--fs-sm);font-weight:600;display:flex;justify-content:space-between}
    header span{color:var(--muted);font-weight:400}
    .s{padding:var(--sp-4);overflow:auto;flex:1;word-break:break-word}
  `;
  render() {
    const style = `font-family:${cssFamilyFor(this.font)};font-size:${this.size}px;font-feature-settings:${featureSettings(this.features)}`;
    return html`<header>${this.font.family}<span>${this.font.classification ?? ''}</span></header><div class="s" style=${style}>${this.text}</div>`;
  }
}
