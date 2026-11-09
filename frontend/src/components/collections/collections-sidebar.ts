import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { libraryContext, collectionsContext, activationContext } from '../../state/contexts';
import type { LibraryStore, Scope } from '../../state/library-store';
import type { CollectionsStore } from '../../state/collections-store';
import type { ActivationStore } from '../../state/activation-store';
import { StoreConnector } from '../../state/store-connector';
import { isFontDrag, draggedFonts } from '../../state/font-drag';
import type { Collection, NewCollection } from '../../data/types';
import '../shared/ui-toggle';
import './collection-dialog';

@customElement('collections-sidebar')
export class CollectionsSidebar extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  @consume({ context: collectionsContext, subscribe: true }) cols!: CollectionsStore;
  @consume({ context: activationContext, subscribe: true }) act!: ActivationStore;
  readonly connector = new StoreConnector(this, () => [this.lib, this.cols, this.act]);
  @state() dialogOpen = false;
  @state() private editing: Collection | null = null;
  @state() private dropTarget: number | null = null;
  /** True while a bulk on/off of every available font is running. */
  @state() private bulkBusy = false;

  static styles = css`
    :host{display:block;height:100%;overflow:auto;background:var(--panel-2);padding:var(--sp-4) var(--sp-2)}
    .lbl{font-size:var(--fs-xs);letter-spacing:.12em;text-transform:uppercase;color:var(--muted);margin:0 var(--sp-2) var(--sp-2)}
    .item{display:flex;align-items:center;gap:var(--sp-2);padding:7px var(--sp-2);border-radius:var(--r-sm);
      font-size:var(--fs-md);cursor:pointer;color:var(--text)}
    .item:hover{background:var(--panel-3)} .item.on{background:var(--accent);color:var(--accent-text)}
    .swatch{width:9px;height:9px;border-radius:3px;flex:none} .grow{flex:1}
    .sep{height:1px;background:var(--border);margin:var(--sp-3) var(--sp-2)}
    .new{color:var(--accent);font-size:var(--fs-sm);padding:7px var(--sp-2);cursor:pointer}
    .item.drop{outline:2px dashed var(--accent);outline-offset:-2px}
    .spin{flex:none;width:34px;height:19px;display:flex;align-items:center;justify-content:center}
    .spin::before{content:"";width:14px;height:14px;border-radius:50%;border:2px solid currentColor;border-right-color:transparent;
      opacity:.7;animation:spin .8s linear infinite}
    @keyframes spin{to{transform:rotate(360deg)}}
    .name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}
    .count{flex:none;min-width:3ch;text-align:right;font-size:var(--fs-xs);color:var(--muted);font-variant-numeric:tabular-nums}
    .item.on .count{color:inherit;opacity:.8}
    .del{display:none;border:0;background:none;color:inherit;opacity:.7;cursor:pointer;font-size:14px;padding:0 4px}
    .item:hover .del, .item:hover .edit{display:inline} .del:hover, .edit:hover{opacity:1}
    .edit{display:none;border:0;background:none;color:inherit;opacity:.7;cursor:pointer;font-size:13px;padding:0 4px}
  `;
  /** True when every activatable library font is on (system fonts never count). */
  private get allOn() {
    const n = this.lib.scopeCounts.activatable;
    return n > 0 && this.act.activeCount >= n;
  }
  private async toggleAll() {
    if (this.bulkBusy) return;
    const turnOn = !this.allOn;
    if (turnOn && !window.confirm(`Turn on all ${this.lib.scopeCounts.activatable} available fonts? Loading many fonts can slow Windows and your other apps.`)) return;
    this.bulkBusy = true;
    try { await this.act.setAllActive(turnOn); } catch (err) { window.alert(`Couldn't change fonts: ${err}`); }
    try {
      await this.cols.refresh();
      if (this.lib.scope === 'active') await this.lib.refresh();
    } finally { this.bulkBusy = false; }
  }
  private pick(s: Scope) { this.lib.setScope(s); }

  private onDragOver(e: DragEvent, cid: number) {
    if (!isFontDrag(e.dataTransfer)) return;
    e.preventDefault();
    e.dataTransfer!.dropEffect = 'copy';
    this.dropTarget = cid;
  }
  private async onDrop(e: DragEvent, cid: number) {
    e.preventDefault();
    this.dropTarget = null;
    const ids = draggedFonts(e.dataTransfer);
    if (!ids.length) return;
    await this.cols.addFonts(cid, ids);
    await this.act.refresh();
    if (this.lib.scope === cid) await this.lib.refresh();
  }
  private async save(e: CustomEvent<{ name: string; color: string }>) {
    const c = this.editing;
    this.editing = null;
    if (!c) return;
    try { await this.cols.update(c, e.detail); } catch (err) { window.alert(`Couldn't save: ${err}`); }
  }
  private async deleteCollection(e: Event, cid: number, name: string) {
    e.stopPropagation();
    if (!window.confirm(`Delete the collection "${name}"? Its fonts stay in your library.`)) return;
    await this.cols.remove(cid);
    await this.act.refresh();
    if (this.lib.scope === cid) this.lib.setScope('all');
  }
  private async create(e: CustomEvent<NewCollection & { includeSelected: boolean }>) {
    const { includeSelected, ...input } = e.detail;
    this.dialogOpen = false;
    try {
      await this.cols.create({ ...input, fontIds: includeSelected ? this.lib.selectedIds : [] });
    } catch (err) { window.alert(`Couldn't create collection: ${err}`); }
  }
  render() {
    const sc = this.lib.scope;
    return html`
      <div class="lbl">Library</div>
      <div class="item ${sc === 'system' ? 'on' : ''}" @click=${() => this.pick('system')}><span class="swatch" style="background:var(--muted)"></span>System fonts <span class="grow"></span><span class="count">${this.lib.scopeCounts.system}</span></div>
      <div class="item ${sc === 'all' ? 'on' : ''}" @click=${() => this.pick('all')}><span class="swatch" style="background:var(--muted)"></span>Available fonts <span class="grow"></span><span class="count">${this.lib.scopeCounts.available}</span>
        ${this.bulkBusy
          ? html`${this.act.bulk?.total ? html`<span class="count">${this.act.bulk.done}/${this.act.bulk.total}</span>` : ''}<span class="spin" role="status" aria-label="Updating fonts" title="Updating fonts…"></span>`
          : html`<ui-toggle title="Turn all available fonts on or off" ?on=${this.allOn} ?disabled=${this.lib.scopeCounts.activatable === 0}
            @toggle=${(e: Event) => { e.stopPropagation(); void this.toggleAll(); }}></ui-toggle>`}</div>
      <div class="item ${sc === 'active' ? 'on' : ''}" @click=${() => this.pick('active')}><span class="swatch" style="background:var(--muted)"></span>Active <span class="grow"></span><span class="count">${this.act.activeCount}</span></div>
      <div class="sep"></div>
      <div class="lbl">Collections</div>
      ${this.cols.collections.map((c) => html`
        <div class="item ${sc === c.id ? 'on' : ''} ${this.dropTarget === c.id ? 'drop' : ''}"
          title="${c.fontIds.length} font${c.fontIds.length === 1 ? '' : 's'} · drop fonts here to add · double-click to edit"
          @click=${() => this.pick(c.id)} @dblclick=${() => (this.editing = c)}
          @dragover=${(e: DragEvent) => this.onDragOver(e, c.id)}
          @dragleave=${() => { if (this.dropTarget === c.id) this.dropTarget = null; }}
          @drop=${(e: DragEvent) => this.onDrop(e, c.id)}>
          <span class="swatch" style="background:${c.color}"></span><span class="name">${c.name}</span><span class="grow"></span>
          <button class="edit" title="Edit collection" aria-label="Edit collection" @click=${(e: Event) => { e.stopPropagation(); this.editing = c; }}>✎</button>
          <button class="del" title="Delete collection" aria-label="Delete collection" @click=${(e: Event) => this.deleteCollection(e, c.id, c.name)}>×</button>
          <span class="count">${c.fontIds.length}</span>
          <ui-toggle ?on=${c.active} @toggle=${(e: Event) => { e.stopPropagation(); this.act.setCollectionActive(c.id, !c.active).then(() => this.cols.refresh()); }}></ui-toggle>
        </div>`)}
      <div class="new" @click=${() => (this.dialogOpen = true)}>+ New collection</div>
      <collection-dialog ?open=${this.dialogOpen || this.editing !== null} .collection=${this.editing} .selectedCount=${this.lib.selectedIds.length}
        @close=${() => { this.dialogOpen = false; this.editing = null; }} @create=${this.create} @save=${this.save}></collection-dialog>
    `;
  }
}
