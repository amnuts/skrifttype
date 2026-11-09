import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { Task } from '@lit/task';
import { libraryContext, preferencesContext } from '../../state/contexts';
import type { LibraryStore } from '../../state/library-store';
import type { PreferencesStore } from '../../state/preferences-store';
import { dataSource } from '../../data';
import { cssForStyle } from '../../services/font-face';
import { hasInstallableStyle, isActivatable, WEB_FONT_TOOLTIP } from '../../services/activatable';
import { featureSettings, styleCountLabel, styleLabels } from '../../services/format';
import type { FontRecord, InstallResult, OpenTypeFeature } from '../../data/types';
import { StoreConnector } from '../../state/store-connector';
import './preview-controls';
import './feature-toggles';
import { isConvertible } from '../convert/convert-dialog';
import '../shared/ui-button';

// Whether this build can install for all users never changes at runtime.
let canInstall: Promise<boolean> | undefined;
const canInstallSystemWide = () => (canInstall ??= dataSource.canInstallSystemWide().catch(() => false));

function describe(verb: string, r: InstallResult): string {
  const lines = [];
  if (r.succeeded.length) lines.push(`${verb}: ${r.succeeded.join(', ')}`);
  for (const f of r.failed) lines.push(`${f.file || 'Error'}: ${f.error}`);
  return lines.join('\n') || 'Nothing to do.';
}

@customElement('preview-pane')
export class PreviewPane extends LitElement {
  @consume({ context: libraryContext, subscribe: true }) lib!: LibraryStore;
  @consume({ context: preferencesContext, subscribe: true }) prefs!: PreferencesStore;
  readonly connector = new StoreConnector(this, () => [this.lib, this.prefs]);
  @state() leading = 110; @state() tracking = 0;
  @state() features = new Set<string>(['liga']);
  @state() private canInstall = false;
  @state() private busy = false;
  @state() private converting = false;
  /** Index into the family's styles, or 'all' for the stacked list. */
  @state() private styleSel: number | 'all' = 0;
  private styleFamilyId: number | undefined;

  protected willUpdate() {
    const id = this.lib.primary?.id;
    if (id !== this.styleFamilyId) { this.styleFamilyId = id; this.styleSel = 0; }
  }

  connectedCallback() {
    super.connectedCallback();
    void canInstallSystemWide().then((ok) => { this.canInstall = ok; });
  }

  private async installForEveryone(f: FontRecord) {
    const ok = window.confirm(
      `Install "${f.family}" for all users?\n\n` +
      'The font files are copied into the Windows Fonts folder so every account and app can use them, ' +
      'even when skrifttype is closed. Windows will ask for administrator permission.');
    if (!ok) return;
    this.busy = true;
    try { window.alert(describe('Installed for everyone', await dataSource.installForEveryone(f.id))); }
    catch (err) { window.alert(String(err instanceof Error ? err.message : err)); }
    finally { this.busy = false; }
  }

  private async uninstallForEveryone(f: FontRecord) {
    const ok = window.confirm(
      `Uninstall "${f.family}" for all users?\n\n` +
      'It is removed from the Windows Fonts folder. Your library copy (if any) is kept. ' +
      'Windows will ask for administrator permission.');
    if (!ok) return;
    this.busy = true;
    try { window.alert(describe('Uninstalled', await dataSource.uninstallForEveryone(f.id))); }
    catch (err) { window.alert(String(err instanceof Error ? err.message : err)); }
    finally { this.busy = false; }
  }

  private renderSystemActions(f: FontRecord) {
    if (!this.canInstall) return '';
    if (f.source === 'library' && f.available && isActivatable(f) && hasInstallableStyle(f)) {
      return html`<ui-button variant="subtle" ?disabled=${this.busy} @click=${() => this.installForEveryone(f)}
        title="Copy into the Windows Fonts folder for all users (asks for administrator permission)">Install for everyone…</ui-button>`;
    }
    if (f.installedByApp) {
      return html`<ui-button variant="ghost" ?disabled=${this.busy} @click=${() => this.uninstallForEveryone(f)}
        title="Remove from the Windows Fonts folder (asks for administrator permission)">Uninstall for everyone…</ui-button>`;
    }
    return '';
  }

  private renderConvertAction(f: FontRecord) {
    if (f.source !== 'library' || !f.available || !isConvertible(f)) return '';
    return html`<ui-button variant="subtle" @click=${() => { this.converting = true; }}
      title="Write a plain TTF/OTF copy of this font to a folder you choose">Convert...</ui-button>`;
  }

  static styles = css`
    :host{display:flex;flex-direction:column;height:100%;overflow:auto}
    .head{padding:var(--sp-5) var(--sp-5) 0} .kicker{font-size:var(--fs-xs);text-transform:uppercase;letter-spacing:.12em;color:var(--muted)}
    .name{font-size:var(--fs-lg);font-weight:600;margin:var(--sp-1) 0 var(--sp-4)}
    .sample{padding:0 var(--sp-5)} .big{user-select:text;margin:0 0 var(--sp-3);color:var(--text);word-break:break-word}
    .foot{margin-top:auto;padding:var(--sp-5);border-top:1px solid var(--border);display:grid;gap:var(--sp-4)}
    .meta{font-size:var(--fs-xs);color:var(--muted);display:flex;gap:var(--sp-3);flex-wrap:wrap}
    .empty{margin:auto;color:var(--muted)}
    .actions{display:flex;gap:var(--sp-2);flex-wrap:wrap}
    .styles{display:flex;gap:var(--sp-2);flex-wrap:wrap;padding:0 var(--sp-5) var(--sp-4)}
    .chip{border:1px solid var(--border);background:var(--panel-2);color:var(--text);border-radius:999px;padding:3px 12px;font-size:var(--fs-sm);cursor:pointer}
    .chip:hover{border-color:var(--accent)} .chip.on{background:var(--accent);border-color:var(--accent);color:var(--accent-text)}
    .stylename{font-size:var(--fs-xs);color:var(--muted);margin:var(--sp-3) 0 var(--sp-1)}
  `;
  private featTask = new Task(this, {
    task: async ([id]) => (id ? await dataSource.getFeatures(id) : []) as OpenTypeFeature[],
    args: () => [this.lib.primary?.id],
  });
  render() {
    const f = this.lib.primary;
    if (!f) return html`<div class="empty">Select a font to preview.</div>`;
    const sel = this.styleSel === 'all' ? undefined : f.styles[this.styleSel] ?? f.styles[0];
    const layout = `font-size:${this.prefs.previewSize}px;line-height:${this.leading}%;letter-spacing:${this.tracking / 1000}em;font-feature-settings:${featureSettings(this.features)}`;
    const multi = f.styles.length > 1;
    const labels = styleLabels(f.styles);
    const sample = this.prefs.sampleText;
    return html`
      <div class="head"><div class="kicker">${f.classification ? `${f.classification} · ` : ''}${styleCountLabel(f.styleCount)}${f.isVariable ? ' · variable' : ''}</div>
        <div class="name">${f.family}</div>
        ${f.source === 'library' && !isActivatable(f) ? html`<div class="kicker" title=${WEB_FONT_TOOLTIP}>Web font: can't be activated on Windows</div>` : ''}</div>
      ${multi ? html`<div class="styles" role="group" aria-label="Styles">
        <button class="chip ${this.styleSel === 'all' ? 'on' : ''}" @click=${() => (this.styleSel = 'all')}>All styles</button>
        ${f.styles.map((_s, i) => html`<button class="chip ${this.styleSel === i ? 'on' : ''}" @click=${() => (this.styleSel = i)}>${labels[i]}</button>`)}</div>` : ''}
      <div class="sample">
        ${this.styleSel === 'all' && multi
          ? f.styles.map((s, i) => html`<div class="stylename">${labels[i]}</div>
              <div class="big" style="${cssForStyle(f, s)};${layout}">${sample}</div>`)
          : html`<div class="big" style="${cssForStyle(f, sel ?? f.styles[0])};${layout}">${sample}</div>`}
      </div>
      <div class="foot">
        <preview-controls .size=${this.prefs.previewSize} .leading=${this.leading} .tracking=${this.tracking}
          @size=${(e: CustomEvent<number>) => this.prefs.setPreviewSize(e.detail)}
          @leading=${(e: CustomEvent<number>) => (this.leading = e.detail)}
          @tracking=${(e: CustomEvent<number>) => (this.tracking = e.detail)}></preview-controls>
        ${this.featTask.render({ complete: (feats) => html`<feature-toggles .features=${feats} .active=${this.features}
          @toggle-feature=${(e: CustomEvent<string>) => { const next = new Set(this.features); next.has(e.detail) ? next.delete(e.detail) : next.add(e.detail); this.features = next; }}></feature-toggles>` })}
        <div class="actions">${this.renderConvertAction(f)}${this.renderSystemActions(f)}</div>
        <div class="meta">${f.designer ? html`<span>Designer: ${f.designer}</span>` : ''}<span>Format: ${f.format.toUpperCase()}</span>${f.parseError ? html`<span style="color:var(--danger)">Parse warning</span>` : ''}</div>
      </div>
      <convert-dialog ?open=${this.converting} .font=${f} @close=${() => { this.converting = false; }}></convert-dialog>`;
  }
}
