import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import type { WatchedDir } from '../../data/types';
import '../shared/ui-toggle';
import '../shared/ui-icon';

@customElement('watched-dirs-editor')
export class WatchedDirsEditor extends LitElement {
  @property({ attribute: false }) dirs: WatchedDir[] = [];
  static styles = css`
    *{box-sizing:border-box}
    .row{display:flex;align-items:center;gap:var(--sp-3);padding:var(--sp-2) 0;border-bottom:1px solid var(--border)}
    .path{flex:1;min-width:0;font-size:var(--fs-sm);font-family:Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .muted{color:var(--muted);font-size:var(--fs-xs)} button{background:none;border:none;color:var(--muted);cursor:pointer}
    .add{margin-top:var(--sp-3);color:var(--accent);font-size:var(--fs-sm);cursor:pointer;background:none;border:none}
  `;
  private fire(name: string, detail: unknown) { this.dispatchEvent(new CustomEvent(name, { detail, bubbles: true, composed: true })); }
  render() {
    return html`
      ${this.dirs.map((d) => html`<div class="row">
        <ui-toggle ?on=${d.enabled} @toggle=${() => this.fire('toggle-dir', d.id)}></ui-toggle>
        <div class="path">${d.path}<div class="muted">${d.recursive ? 'recursive' : 'top level only'}</div></div>
        <button title="Remove" @click=${() => this.fire('remove-dir', d.id)}><ui-icon name="close" size="16"></ui-icon></button>
      </div>`)}
      <button class="add" @click=${() => this.fire('add-dir', null)}>+ Add folder…</button>`;
  }
}
