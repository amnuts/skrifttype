import { LitElement, html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { preferencesContext } from '../../state/contexts';
import type { PreferencesStore } from '../../state/preferences-store';
import { dataSource } from '../../data';
import type { WatchedDir } from '../../data/types';
import { StoreConnector } from '../../state/store-connector';
import '../shared/ui-dialog';
import '../shared/ui-button';
import './watched-dirs-editor';

@customElement('settings-dialog')
export class SettingsDialog extends LitElement {
  @consume({ context: preferencesContext, subscribe: true }) prefs!: PreferencesStore;
  readonly connector = new StoreConnector(this, () => [this.prefs]);
  @property({ type: Boolean, reflect: true }) open = false;
  @state() dirs: WatchedDir[] = [];
  @state() convertFolder = '';
  static styles = css`
    *{box-sizing:border-box}
    .field{margin-bottom:var(--sp-5)} label{display:block;font-size:var(--fs-sm);color:var(--muted);margin-bottom:var(--sp-2)}
    input{box-sizing:border-box;width:100%;height:34px;padding:0 var(--sp-3);border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel-2);color:var(--text)}
    .replay{margin-top:var(--sp-4);padding-top:var(--sp-4);border-top:1px solid var(--border)}
    .row{display:flex;align-items:center;gap:var(--sp-3)}
    .path{flex:1;min-width:0;font-size:var(--fs-sm);font-family:Consolas,monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .link{background:none;border:none;color:var(--accent);cursor:pointer;font-size:var(--fs-sm);padding:0}
  `;
  async updated(changed: Map<string, unknown>) {
    if (changed.has('open') && this.open) await this.refresh();
  }
  private async refresh() {
    this.dirs = await dataSource.listWatchedRoots();
    this.convertFolder = await dataSource.getConvertFolder();
  }
  private async browseConvertFolder() {
    const path = await dataSource.pickFolder();
    if (!path) return;
    await dataSource.setConvertFolder(path);
    this.convertFolder = path;
  }
  private async clearConvertFolder() {
    await dataSource.setConvertFolder('');
    this.convertFolder = '';
  }
  private async addDir() {
    const path = await dataSource.pickFolder();
    if (!path) return;
    await dataSource.addWatchedRoot(path, true);
    await this.refresh();
  }
  private async toggleDir(id: number) {
    const d = this.dirs.find((x) => x.id === id);
    if (!d) return;
    await dataSource.setWatchedRootEnabled(id, !d.enabled);
    await this.refresh();
  }
  private async removeDir(id: number) { await dataSource.removeWatchedRoot(id); await this.refresh(); }
  private close() { this.dispatchEvent(new CustomEvent('close', { bubbles: true, composed: true })); }
  private replay() {
    this.close();
    this.dispatchEvent(new CustomEvent('replay-onboarding', { bubbles: true, composed: true }));
  }
  render() {
    return html`<ui-dialog ?open=${this.open} heading="Settings" @close=${this.close}>
      <div class="field"><label>Watched folders</label>
        <watched-dirs-editor .dirs=${this.dirs}
          @toggle-dir=${(e: CustomEvent<number>) => this.toggleDir(e.detail)}
          @remove-dir=${(e: CustomEvent<number>) => this.removeDir(e.detail)}
          @add-dir=${() => this.addDir()}></watched-dirs-editor></div>
      <div class="field"><label>Conversion folder</label>
        <div class="row"><div class="path" title=${this.convertFolder}>${this.convertFolder || 'Not set (asked when you convert)'}</div>
          <ui-button variant="subtle" @click=${this.browseConvertFolder}>Browse...</ui-button>
          <ui-button variant="ghost" ?disabled=${!this.convertFolder} @click=${this.clearConvertFolder}>Clear</ui-button></div></div>
      <div class="replay"><button class="link" @click=${this.replay}>↺ Replay the walkthrough</button></div>
    </ui-dialog>`;
  }
}
