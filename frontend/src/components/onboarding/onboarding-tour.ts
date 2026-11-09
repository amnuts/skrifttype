import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { TourStep } from '../../state/onboarding-controller';
import '../shared/ui-button';

@customElement('onboarding-tour')
export class OnboardingTour extends LitElement {
  @property({ type: Boolean, reflect: true }) open = false;
  @property({ attribute: false }) step?: TourStep;
  @property({ type: Number }) index = 0;
  @property({ type: Number }) total = 0;
  @property({ attribute: false }) rect: DOMRect | null = null;

  static styles = css`
    :host{display:none} :host([open]){display:block}
    /* Layer ignores pointer events so the spotlighted UI beneath (e.g. the
       Settings "Add folder…" button in step 2) stays clickable; only the card
       and the centered scrim are interactive. */
    .layer{position:fixed;inset:0;z-index:80;pointer-events:none}
    /* The spotlight dims everything except its own box via a huge shadow. */
    .spot{position:fixed;border-radius:var(--r-md);box-shadow:0 0 0 9999px rgba(10,15,25,.62);
      transition:all 180ms var(--ease);pointer-events:none}
    .scrim-center{position:fixed;inset:0;background:rgba(10,15,25,.62);pointer-events:auto}
    .card{position:fixed;width:320px;max-width:92vw;background:var(--panel);color:var(--text);
      border:1px solid var(--border);border-radius:var(--r-lg);box-shadow:var(--shadow);padding:var(--sp-5);z-index:81;pointer-events:auto}
    h3{margin:0 0 var(--sp-2);font-size:var(--fs-lg)}
    p{margin:0 0 var(--sp-4);font-size:var(--fs-md);color:var(--text);line-height:1.5}
    .dots{display:flex;gap:6px;margin-bottom:var(--sp-4)}
    .dot{width:6px;height:6px;border-radius:50%;background:var(--border)} .dot.on{background:var(--accent)}
    .row{display:flex;align-items:center;justify-content:space-between;gap:var(--sp-2)}
    .right{display:flex;gap:var(--sp-2)}
    .skip{background:none;border:none;color:var(--muted);cursor:pointer;font-size:var(--fs-sm)}
  `;

  private fire(name: string) { this.dispatchEvent(new CustomEvent(name, { bubbles: true, composed: true })); }

  private cardStyle(): string {
    const pad = 8, cardW = 320, cardH = 200, gap = 14;
    const vw = window.innerWidth, vh = window.innerHeight;
    if (!this.rect) {
      return `left:${Math.round((vw - cardW) / 2)}px;top:${Math.round((vh - cardH) / 2)}px`;
    }
    const r = this.rect;
    // Prefer right of target, else left, else below, else above.
    let left = r.right + gap, top = r.top;
    if (left + cardW > vw - pad) left = r.left - gap - cardW;
    if (left < pad) { left = Math.min(Math.max(pad, r.left), vw - cardW - pad); top = r.bottom + gap; }
    if (top + cardH > vh - pad) top = Math.max(pad, vh - cardH - pad);
    if (top < pad) top = pad;
    return `left:${Math.round(left)}px;top:${Math.round(top)}px`;
  }

  private spotStyle(): string {
    if (!this.rect) return 'display:none';
    const p = 6, r = this.rect;
    return `left:${Math.round(r.left - p)}px;top:${Math.round(r.top - p)}px;width:${Math.round(r.width + p * 2)}px;height:${Math.round(r.height + p * 2)}px`;
  }

  render() {
    if (!this.step) return html``;
    const isLast = this.index >= this.total - 1;
    return html`<div class="layer">
      ${this.rect ? html`<div class="spot" style=${this.spotStyle()}></div>` : html`<div class="scrim-center"></div>`}
      <div class="card" style=${this.cardStyle()}>
        <div class="dots">${Array.from({ length: this.total }, (_, i) =>
          html`<span class="dot ${i === this.index ? 'on' : ''}"></span>`)}</div>
        <h3>${this.step.title}</h3>
        <p>${this.step.body}</p>
        <div class="row">
          <button class="skip" @click=${() => this.fire('skip')}>Skip</button>
          <div class="right">
            ${this.index > 0 ? html`<ui-button variant="subtle" @click=${() => this.fire('back')}>Back</ui-button>` : ''}
            <ui-button variant="solid" @click=${() => this.fire(isLast ? 'finish' : 'next')}>${isLast ? 'Done' : 'Next'}</ui-button>
          </div>
        </div>
      </div>
    </div>`;
  }
}
