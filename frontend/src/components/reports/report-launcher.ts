import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { Task } from '@lit/task';
import { libraryContext, collectionsContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import type { CollectionsStore } from '../../state/collections-store';
import { dataSource } from '../../data';
import type { ReportType } from '../../data/types';
import { openReport } from './report-window';
import { StoreConnector } from '../../state/store-connector';
import '../shared/ui-button';
import './specimen-report';
import './overview-report';

@customElement('report-launcher')
export class ReportLauncher extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  @consume({ context: collectionsContext, subscribe: true }) cols!: CollectionsStore;
  readonly connector = new StoreConnector(this, () => [this.lib, this.cols]);
  @state() type: ReportType = 'specimen';
  @state() scope: 'all' | 'system' | 'active' | number = 'active';
  @state() text = 'Typography that breathes.';
  static styles = css`
    :host{display:flex;flex-direction:column;height:100%}
    .controls{padding:var(--sp-4);border-bottom:1px solid var(--border);display:grid;gap:var(--sp-3)}
    .group{display:flex;gap:var(--sp-2);flex-wrap:wrap} label{font-size:var(--fs-xs);color:var(--muted);text-transform:uppercase;letter-spacing:.1em}
    select,input{height:32px;padding:0 var(--sp-3);border:1px solid var(--border);border-radius:var(--r-md);background:var(--panel-2);color:var(--text)}
    .preview{flex:1;overflow:auto;padding:var(--sp-4)}
  `;
  private fontsTask = new Task(this, {
    task: async ([s]) => dataSource.listFonts({ scope: s }),
    args: () => [this.scope] as const,
  });
  private async open() {
    const data = await dataSource.buildReport({ type: this.type, scope: this.scope, sampleText: this.text });
    await openReport(data);
  }
  render() {
    return html`
      <div class="controls">
        <div><label>Report type</label><div class="group">
          <ui-button ?active=${this.type === 'specimen'} @click=${() => (this.type = 'specimen')}>Specimen</ui-button>
          <ui-button ?active=${this.type === 'overview'} @click=${() => (this.type = 'overview')}>Overview</ui-button></div></div>
        <div><label>Scope</label><br/>
          <select @change=${(e: Event) => { const v = (e.target as HTMLSelectElement).value; this.scope = v === 'all' || v === 'system' || v === 'active' ? v : Number(v); }}>
            <option value="active">Active fonts</option><option value="all">Available fonts</option><option value="system">System fonts</option>
            ${this.cols.collections.map((c) => html`<option value=${c.id}>${c.name}</option>`)}
          </select></div>
        <div><label>Sample text</label><br/><input style="width:100%;box-sizing:border-box" .value=${this.text} @input=${(e: Event) => (this.text = (e.target as HTMLInputElement).value)} /></div>
        <ui-button variant="solid" @click=${this.open}>Open report ↗ (Save as PDF)</ui-button>
      </div>
      <div class="preview">${this.fontsTask.render({ complete: (fonts) =>
        this.type === 'specimen'
          ? html`<specimen-report .fonts=${fonts} .text=${this.text}></specimen-report>`
          : html`<overview-report .fonts=${fonts} .text=${this.text}></overview-report>` })}</div>`;
  }
}
