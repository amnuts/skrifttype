import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { dataSource } from '../../data';
import type { ConvertResult, FontRecord } from '../../data/types';
import { hasCollectionStyles, hasConvertibleStyles, planConversion, type CollectionScope } from './convert-plan';
import '../shared/ui-dialog';
import '../shared/ui-button';

/** The Convert action applies when at least one style is a web font or collection file. */
export const isConvertible = hasConvertibleStyles;

type Labelled = ConvertResult & { label: string };

@customElement('convert-dialog')
export class ConvertDialog extends LitElement {
  @property({ type: Boolean, reflect: true }) open = false;
  @property({ attribute: false }) font: FontRecord | null = null;
  @state() private scope: CollectionScope = 'family';
  private session = 0;
  @state() private folder = '';        // saved conversion folder ("" when unset)
  @state() private chosen = '';        // folder picked in this dialog
  @state() private changing = false;   // user clicked Change on a saved folder
  @state() private remember = true;
  @state() private busy = false;
  @state() private results: Labelled[] | null = null;
  @state() private notWatched = false;
  @state() private error = '';

  static styles = css`
    *{box-sizing:border-box}
    .field{margin-bottom:var(--sp-4)} .lbl{display:block;font-size:var(--fs-sm);color:var(--muted);margin-bottom:var(--sp-2)}
    .row{display:flex;align-items:center;gap:var(--sp-3)}
    .path{flex:1;min-width:0;font-size:var(--fs-sm);font-family:Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .check{display:flex;align-items:center;gap:var(--sp-2);font-size:var(--fs-sm);color:var(--text);margin-top:var(--sp-2)}
    .check input{width:auto;height:auto}
    .note{font-size:var(--fs-xs);color:var(--muted);margin:var(--sp-4) 0}
    .link{background:none;border:none;color:var(--accent);cursor:pointer;font-size:var(--fs-sm);padding:0}
    .actions{display:flex;justify-content:flex-end;gap:var(--sp-2);margin-top:var(--sp-5)}
    ul{margin:0 0 var(--sp-3);padding-left:var(--sp-5);font-size:var(--fs-sm)} li{margin-bottom:var(--sp-1);word-break:break-all}
    .err{color:var(--danger)}
  `;

  protected willUpdate(changed: Map<string, unknown>) {
    if (changed.has('open') && this.open) {
      this.session++;
      const session = this.session;
      this.scope = 'family'; this.chosen = ''; this.changing = false; this.remember = true;
      this.busy = false; this.results = null; this.notWatched = false; this.error = '';
      this.folder = '';
      void dataSource.getConvertFolder()
        .then((d) => { if (session === this.session) this.folder = d; })
        .catch(() => { if (session === this.session) this.folder = ''; });
    }
  }

  private get plan() { return this.font ? planConversion(this.font, this.scope) : []; }
  private get hasCollection() { return !!this.font && hasCollectionStyles(this.font); }
  private get targetDir() { return this.needsFolder ? this.chosen : ''; }
  private get needsFolder() { return !this.folder || this.changing; }

  private close() {
    if (this.busy) return;
    this.dispatchEvent(new CustomEvent('close', { bubbles: true, composed: true }));
  }
  private async browse() {
    const p = await dataSource.pickFolder();
    if (p) this.chosen = p;
  }
  private async convert() {
    const f = this.font;
    if (!f || this.busy) return;
    const calls = this.plan;
    if (calls.length === 0) { this.error = 'Nothing to convert.'; return; }
    const dir = this.targetDir;
    if (this.needsFolder && !dir) { this.error = 'Choose a folder first.'; return; }
    const session = this.session;
    const live = () => session === this.session;
    this.error = ''; this.busy = true;
    const out: Labelled[] = [];
    let failure = '';
    try {
      for (let i = 0; i < calls.length; i++) {
        const c = calls[i];
        try {
          const res = await dataSource.convertFont(c.styleId, c.allFaces, dir, i === 0 && dir !== '' && this.remember);
          if (!live()) return;
          for (const r of res) out.push({ ...r, label: c.label });
        } catch (err) {
          if (!live()) return;
          out.push({ faceIndex: 0, path: '', err: String(err instanceof Error ? err.message : err), label: c.label });
        }
      }
    } catch (err) {
      failure = String(err instanceof Error ? err.message : err);
    }
    if (!live()) return;
    const outDir = dir || this.folder;
    let notWatched = false;
    if (out.some((r) => !r.err) && outDir) {
      try { notWatched = !(await dataSource.inWatchedRoot(outDir)); } catch { notWatched = false; }
      if (!live()) return;
    }
    if (dir && this.remember && out.some((r) => !r.err)) this.folder = dir;
    this.notWatched = notWatched;
    this.busy = false;
    if (out.length > 0) { this.results = out; this.error = failure; }
    else this.error = failure || 'Nothing was converted.';
  }

  private summary() {
    const calls = this.plan;
    const n = calls.length;
    if (this.hasCollection && this.scope === 'family' && calls.every((c) => !c.allFaces)) {
      return `Converts ${n} face${n === 1 ? '' : 's'}`;
    }
    return `Converts ${n} file${n === 1 ? '' : 's'}`;
  }

  private renderFolder() {
    if (!this.needsFolder) {
      return html`<div class="field"><span class="lbl">Save converted fonts to</span>
        <div class="row"><div class="path" title=${this.folder}>${this.folder}</div>
          <button class="link" @click=${() => { this.changing = true; }}>Change</button></div></div>`;
    }
    return html`<div class="field"><span class="lbl">Save converted fonts to</span>
      <div class="row"><div class="path" title=${this.chosen}>${this.chosen || 'No folder chosen'}</div>
        <ui-button variant="subtle" @click=${this.browse}>Browse...</ui-button></div>
      <label class="check"><input type="checkbox" .checked=${this.remember}
        @change=${(e: Event) => (this.remember = (e.target as HTMLInputElement).checked)} />
        Remember this folder</label></div>`;
  }

  private renderResults(results: Labelled[]) {
    const ok = results.filter((r) => !r.err);
    return html`
      ${ok.length === 0 ? html`<p class="err">Nothing was converted.</p>` : html`<p>Converted ${ok.length} of ${results.length} face${results.length === 1 ? '' : 's'}.</p>`}
      <ul>${results.map((r) => r.err
        ? html`<li class="err">${r.label} (face ${r.faceIndex + 1}): ${r.err}</li>`
        : html`<li>${r.label}: ${r.path}</li>`)}</ul>
      ${this.error ? html`<p class="err">${this.error}</p>` : ''}
      ${this.notWatched ? html`<p class="note">Add this folder as a library folder to see the converted fonts.</p>` : ''}
      <div class="actions"><ui-button variant="solid" @click=${this.close}>Done</ui-button></div>`;
  }

  render() {
    const f = this.font;
    return html`<ui-dialog ?open=${this.open} heading=${f ? `Convert ${f.family}` : 'Convert'} @close=${(e: Event) => { e.stopPropagation(); this.close(); }}>
      ${this.results ? this.renderResults(this.results) : html`
        ${this.hasCollection ? html`<div class="field"><span class="lbl">Faces</span>
          <label class="check"><input type="radio" name="faces" .checked=${this.scope === 'family'} @change=${() => (this.scope = 'family')} />This family's faces</label>
          <label class="check"><input type="radio" name="faces" .checked=${this.scope === 'file'} @change=${() => (this.scope = 'file')} />Every face in the file(s)</label></div>` : ''}
        <p class="note">${this.summary()}</p>
        ${this.renderFolder()}
        <p class="note">Some web fonts are licensed for web use only - check the licence before using a converted copy on the desktop.</p>
        ${this.error ? html`<p class="err">${this.error}</p>` : ''}
        <div class="actions"><ui-button variant="ghost" ?disabled=${this.busy} @click=${this.close}>Cancel</ui-button>
          <ui-button variant="solid" ?disabled=${this.busy || this.plan.length === 0} @click=${this.convert}>${this.busy ? 'Converting...' : 'Convert'}</ui-button></div>`}
    </ui-dialog>`;
  }
}
