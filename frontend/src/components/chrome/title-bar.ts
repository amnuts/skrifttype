import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { consume } from '@lit/context';
import { preferencesContext } from '../../state/contexts';
import type { PreferencesStore } from '../../state/preferences-store';
import { StoreConnector } from '../../state/store-connector';
import '../shared/ui-icon';

@customElement('title-bar')
export class TitleBar extends LitElement {
  @consume({ context: preferencesContext, subscribe: true }) prefs!: PreferencesStore;
  @property({ type: Number }) total = 0;
  @property({ type: Number }) active = 0;
  readonly connector = new StoreConnector(this, () => [this.prefs]);
  static styles = css`
    header{height:42px;display:flex;align-items:center;gap:var(--sp-3);padding:0 var(--sp-4);
      background:var(--panel-2);border-bottom:1px solid var(--border)}
    .mark{width:14px;height:14px;border-radius:5px;background:linear-gradient(135deg,var(--accent),var(--c5))}
    .name{font-weight:700;letter-spacing:.01em}
    .counts{color:var(--muted);font-size:var(--fs-sm)}
    .spacer{flex:1}
    button{background:none;border:none;color:var(--muted);cursor:pointer;padding:6px;border-radius:var(--r-sm)}
    button:hover{background:var(--panel-3);color:var(--text)}
  `;
  render() {
    const dark = this.prefs?.theme === 'dark';
    return html`<header>
      <span class="mark"></span><span class="name">skrifttype</span>
      <span class="counts">${this.total.toLocaleString()} fonts · ${this.active} active</span>
      <span class="spacer"></span>
      <button title="Toggle theme" @click=${() => this.prefs.toggleTheme()}><ui-icon name=${dark ? 'sun' : 'moon'}></ui-icon></button>
      <button title="Settings" @click=${() => this.dispatchEvent(new CustomEvent('open-settings', { bubbles: true, composed: true }))}><ui-icon name="gear"></ui-icon></button>
    </header>`;
  }
}
