import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { Task } from '@lit/task';
import { libraryContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import { dataSource } from '../../data';
import { cssFamilyFor } from '../../services/font-face';
import { copyText } from '../../services/clipboard';
import { codepointHex } from '../../services/format';
import type { Range } from '../../data/types';
import { StoreConnector } from '../../state/store-connector';
import './glyph-cell';

@customElement('glyph-map')
export class GlyphMap extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  readonly connector = new StoreConnector(this, () => [this.lib]);
  @state() toast: { char: string; hex: string } | null = null;
  static styles = css`
    :host{display:flex;flex-direction:column;height:100%}
    .grid{flex:1;overflow:auto;padding:var(--sp-4);display:grid;gap:var(--sp-2);
      grid-template-columns:repeat(auto-fill,minmax(52px,1fr))}
    .toast{position:absolute;bottom:var(--sp-5);left:50%;transform:translateX(-50%);background:var(--text);color:var(--bg);
      padding:var(--sp-2) var(--sp-4);border-radius:var(--r-pill);font-size:var(--fs-sm);box-shadow:var(--shadow)}
    .toast{display:flex;align-items:center;gap:var(--sp-2);white-space:nowrap}
    .toast .ch{font-size:1.4em;line-height:1;min-width:1.4em;text-align:center}
    .empty{margin:auto;color:var(--muted)}
  `;
  private rangesTask = new Task(this, {
    task: async ([id]) => (id ? await dataSource.getGlyphRanges(id) : []) as Range[],
    args: () => [this.lib.primary?.id],
  });
  private expand(ranges: Range[]): number[] {
    const out: number[] = [];
    for (const r of ranges) for (let cp = r.start; cp <= r.end && out.length < 2000; cp++) out.push(cp);
    return out;
  }
  private async onCopy(e: CustomEvent<{ cp: number; char: string }>) {
    await copyText(e.detail.char);
    this.toast = { char: e.detail.char, hex: codepointHex(e.detail.cp) };
    setTimeout(() => (this.toast = null), 1400);
  }
  render() {
    const f = this.lib.primary;
    if (!f) return html`<div class="empty">Select a font to explore its glyphs.</div>`;
    const fam = cssFamilyFor(f);
    return html`${this.rangesTask.render({ complete: (ranges) => html`
      <div class="grid" @copy-glyph=${this.onCopy}>
        ${this.expand(ranges).map((cp) => html`<glyph-cell .cp=${cp} .fontFamily=${fam} .size=${28}></glyph-cell>`)}
      </div>` })}
      ${this.toast ? html`<div class="toast"><span>Copied</span><span class="ch" style="font-family:${fam}">${this.toast.char}</span><span>(${this.toast.hex})</span></div>` : ''}`;
  }
}
