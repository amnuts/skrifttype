import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { FontRecord } from '../../data/types';
import { cssFamilyFor } from '../../services/font-face';

@customElement('overview-report')
export class OverviewReport extends LitElement {
  @property({ attribute: false }) fonts: FontRecord[] = [];
  @property() text = '';
  static styles = css`
    :host{display:block} .row{display:flex;gap:var(--sp-4);align-items:baseline;padding:var(--sp-2) 0;border-bottom:1px solid var(--border)}
    .label{flex:0 0 140px;font-weight:600;font-size:var(--fs-sm)}
  `;
  render() {
    return html`${this.fonts.slice(0, 40).map((f) => html`<div class="row"><div class="label">${f.family}</div>
      <div style="font-family:${cssFamilyFor(f)};font-size:22px">${this.text}</div></div>`)}`;
  }
}
