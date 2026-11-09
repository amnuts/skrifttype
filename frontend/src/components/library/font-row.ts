import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import type { FontRecord } from '../../data/types';
import { cssFamilyFor, fitScale, ROW_LINE } from '../../services/font-face';
import { styleCountLabel } from '../../services/format';
import { isActivatable, WEB_FONT_TOOLTIP } from '../../services/activatable';
import { setDraggedFonts } from '../../state/font-drag';
import '../shared/ui-toggle';

@customElement('font-row')
export class FontRow extends LitElement {
  @property({ attribute: false }) font!: FontRecord;
  /** The phrase each row sets in its own font. */
  @property() sample = '';
  /** Base size (px) of the phrase before per-font adjustment. */
  @property({ type: Number }) size = 17;
  @property({ type: Boolean }) active = false;
  @property({ type: Boolean }) selected = false;
  /** Show a "remove from collection" button (library is scoped to a collection). */
  @property({ type: Boolean }) removable = false;
  @state() private scale = 1;
  private scaledFor = '';
  willUpdate() {
    const text = this.sample || this.font.family;
    const key = `${this.font.cssFamily}|${text}`;
    if (key === this.scaledFor) return;
    this.scaledFor = key;
    this.scale = 1;
    void fitScale(this.font, text).then((k) => { if (this.scaledFor === key) this.scale = k; });
  }
  static styles = css`
    :host{display:block;width:100%}
    .row{display:flex;align-items:center;gap:var(--sp-3);padding:var(--sp-3) var(--sp-4);border-bottom:1px solid var(--border);cursor:pointer}
    .row:hover{background:var(--panel-2)} .row.sel{background:var(--accent-weak)}
    .meta{flex:1;min-width:0} .fam{font-size:calc(var(--base,17px) * var(--fit,1));line-height:calc(var(--base,17px) * ${ROW_LINE});white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .sub{font-size:var(--fs-xs);color:var(--muted);margin-top:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis} .cls{text-transform:capitalize}
    .badge{font-family:"Inter","Segoe UI",system-ui,sans-serif;font-size:9.5px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted);border:1px solid var(--border);border-radius:5px;padding:2px 6px;margin-left:var(--sp-2)}
    .badge.warn{color:var(--danger,#c0392b);border-color:currentColor}
    /* Library fonts that can't be switched on (web fonts, missing files). */
    .row.dim .meta{opacity:.5}
    .rm{border:0;background:none;color:var(--muted);cursor:pointer;font-size:16px;line-height:1;padding:4px 6px;border-radius:var(--r-sm)}
    .rm:hover{background:var(--panel-3);color:var(--text)}
  `;
  private toggle(e: Event) { e.stopPropagation(); this.dispatchEvent(new CustomEvent('toggle-active', { detail: this.font.id, bubbles: true, composed: true })); }
  private removeFromCollection(e: Event) { e.stopPropagation(); this.dispatchEvent(new CustomEvent('remove-from-collection', { detail: this.font.id, bubbles: true, composed: true })); }
  private dragStart(e: DragEvent) {
    if (!e.dataTransfer) return;
    setDraggedFonts(e.dataTransfer, [this.font.id]);
    // Let the pane widen the payload to the whole multi-selection.
    this.dispatchEvent(new CustomEvent('drag-font', { detail: { id: this.font.id, dataTransfer: e.dataTransfer }, bubbles: true, composed: true }));
  }
  private select(e: MouseEvent) { this.dispatchEvent(new CustomEvent('select-font', { detail: { id: this.font.id, multi: e.ctrlKey || e.metaKey || e.shiftKey }, bubbles: true, composed: true })); }
  render() {
    const fam = cssFamilyFor(this.font); const isSystem = this.font.source !== 'library';
    const webOnly = !isSystem && !isActivatable(this.font);
    const missing = !this.font.available;
    return html`<div class="row ${this.selected ? 'sel' : ''} ${missing || webOnly ? 'dim' : ''}" draggable="true"
      @click=${this.select} @dragstart=${this.dragStart}>
      <div class="meta"><div class="fam" style="font-family:${fam};--fit:${this.scale};--base:${this.size}px" title=${this.font.family}>${this.sample || this.font.family}</div>
        <div class="sub">${this.font.family}${this.font.classification ? html` · <span class="cls">${this.font.classification}</span>` : ''} · ${styleCountLabel(this.font.styleCount)} · ${this.font.format}${isSystem ? html`<span class="badge">system</span>` : ''}${missing ? html`<span class="badge warn" title="The font file could not be found">missing</span>` : ''}</div></div>
      ${this.removable ? html`<button class="rm" title="Remove from collection" aria-label="Remove from collection" @click=${this.removeFromCollection}>×</button>` : ''}
      <ui-toggle ?on=${this.active} ?disabled=${isSystem || webOnly || (missing && !this.active)} title=${webOnly ? WEB_FONT_TOOLTIP : nothing} @toggle=${this.toggle}></ui-toggle>
    </div>`;
  }
}
