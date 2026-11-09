import { LitElement, html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { preferencesContext } from '../../state/contexts';
import type { PreferencesStore } from '../../state/preferences-store';
import { DEFAULT_PHRASE } from '../../state/preferences-store';
import { StoreConnector } from '../../state/store-connector';

/** Toolbar pill that shows the preview phrase and opens a menu to pick, add or remove phrases. */
@customElement('phrase-picker')
export class PhrasePicker extends LitElement {
  @consume({ context: preferencesContext, subscribe: true }) prefs!: PreferencesStore;
  @state() private open = false;
  @state() private draft = '';
  readonly connector = new StoreConnector(this, () => [this.prefs]);
  static styles = css`
    :host{position:relative;display:block}
    .pill{display:flex;align-items:center;gap:var(--sp-2);max-width:220px;font-size:var(--fs-sm);padding:5px 11px;border-radius:var(--r-pill);
      border:1px solid var(--border);background:var(--panel-2);color:var(--text);cursor:pointer}
    .pill:hover,.pill[aria-expanded=true]{border-color:var(--accent)}
    .pill span{min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis} .pill i{font-style:normal;color:var(--muted);flex:none}
    .menu{position:absolute;right:0;top:calc(100% + 6px);z-index:20;width:320px;max-width:80vw;background:var(--panel);
      border:1px solid var(--border);border-radius:var(--r-md);box-shadow:0 8px 24px rgba(0,0,0,.25);padding:var(--sp-2)}
    ul{list-style:none;margin:0;padding:0;max-height:260px;overflow:auto}
    li{display:flex;align-items:center;gap:var(--sp-2);border-radius:var(--r-sm)}
    li:hover{background:var(--panel-2)} li.on{background:var(--accent-weak)}
    li .pick{flex:1;min-width:0;text-align:left;border:0;background:none;color:var(--text);font:inherit;font-size:var(--fs-sm);
      padding:6px 8px;cursor:pointer;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}
    li .rm{border:0;background:none;color:var(--muted);cursor:pointer;font-size:16px;line-height:1;padding:4px 8px;border-radius:var(--r-sm)}
    li .rm:hover{background:var(--panel-3);color:var(--text)}
    .def{font-size:var(--fs-xs);color:var(--muted);padding:0 8px}
    form{display:flex;gap:var(--sp-2);margin-top:var(--sp-2);padding-top:var(--sp-2);border-top:1px solid var(--border)}
    input{flex:1;min-width:0;height:28px;padding:0 var(--sp-2);border:1px solid var(--border);border-radius:var(--r-sm);
      background:var(--panel-2);color:var(--text);font-size:var(--fs-sm)}
    form button{border:0;border-radius:var(--r-sm);padding:0 12px;background:var(--accent);color:var(--accent-text);cursor:pointer;font-size:var(--fs-sm)}
    form button:disabled{opacity:.5;cursor:default}
  `;
  private onDoc = (e: Event) => { if (this.open && !e.composedPath().includes(this)) this.open = false; };
  private onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') this.open = false; };
  connectedCallback() { super.connectedCallback(); document.addEventListener('pointerdown', this.onDoc); document.addEventListener('keydown', this.onKey); }
  disconnectedCallback() { super.disconnectedCallback(); document.removeEventListener('pointerdown', this.onDoc); document.removeEventListener('keydown', this.onKey); }
  private add(e: Event) {
    e.preventDefault();
    if (!this.draft.trim()) return;
    this.prefs.addPhrase(this.draft);
    this.draft = '';
  }
  render() {
    if (!this.prefs) return html``;
    const current = this.prefs.sampleText;
    const all = [DEFAULT_PHRASE, ...this.prefs.phrases];
    return html`<button class="pill" aria-haspopup="true" aria-expanded=${this.open} title=${current} @click=${() => { this.open = !this.open; }}>
        <span>${current}</span><i>▾</i></button>
      ${this.open ? html`<div class="menu" role="menu">
        <ul>${all.map((p) => html`<li class=${p === current ? 'on' : ''}>
          <button class="pick" role="menuitemradio" aria-checked=${p === current} title=${p} @click=${() => { this.prefs.selectPhrase(p); this.open = false; }}>${p}</button>
          ${p === DEFAULT_PHRASE ? html`<span class="def">default</span>` : html`<button class="rm" title="Remove phrase" aria-label="Remove phrase" @click=${() => this.prefs.removePhrase(p)}>×</button>`}</li>`)}</ul>
        <form @submit=${this.add}>
          <input placeholder="New phrase…" .value=${this.draft} @input=${(e: Event) => { this.draft = (e.target as HTMLInputElement).value; }} />
          <button type="submit" ?disabled=${!this.draft.trim()}>Add</button></form>
      </div>` : ''}`;
  }
}
