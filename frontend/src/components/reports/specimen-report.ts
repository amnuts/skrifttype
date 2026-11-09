import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { FontRecord } from '../../data/types';
import { cssFamilyFor } from '../../services/font-face';

@customElement('specimen-report')
export class SpecimenReport extends LitElement {
  @property({ attribute: false }) fonts: FontRecord[] = [];
  @property() text = '';
  static styles = css`
    :host{display:block} .card{border:1px solid var(--border);border-radius:var(--r-md);padding:var(--sp-4);margin-bottom:var(--sp-3)}
    .name{font-weight:600;margin-bottom:var(--sp-2)} .s{word-break:break-word}
  `;
  render() {
    return html`${this.fonts.slice(0, 8).map((f) => html`<div class="card"><div class="name">${f.family}</div>
      <div class="s" style="font-family:${cssFamilyFor(f)};font-size:40px">${this.text}</div></div>`)}`;
  }
}
