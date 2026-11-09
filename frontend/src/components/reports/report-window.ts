import type { ReportData } from '../../data/types';
import { cssFamilyFor, shareFontFaces } from '../../services/font-face';

// The report renders in a separate plain window via document.write, so font
// metadata and user sample text must be HTML-escaped to stay markup-safe once
// real (non-mock) strings flow through.
const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const PRINT_CSS = `
  body{font-family:Inter,"Segoe UI",system-ui,sans-serif;color:#1d2433;margin:0;padding:24px;background:#fff}
  h1{font-size:18px;margin:0 0 8px} .toolbar{position:sticky;top:0;background:#fff;padding-bottom:10px;border-bottom:1px solid #e8ecf3;margin-bottom:12px}
  button{padding:6px 12px;border:1px solid #12b3a6;background:#12b3a6;color:#fff;border-radius:8px;cursor:pointer;font-size:13px}
  .note{font-size:11px;color:#b45309;margin-top:8px}
  .specimen{break-inside:avoid;padding:6px 0;border-bottom:1px solid #eef1f7}
  .fam{font-size:9px;color:#9aa3b4;text-transform:uppercase;letter-spacing:.08em;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .name{font-weight:600;color:#1d2433;text-transform:none;letter-spacing:0;margin-right:8px;font-size:10px}
  .sample{margin:0;line-height:1.25;white-space:nowrap;overflow:hidden;text-overflow:clip}
  .overview-row{break-inside:avoid;display:flex;align-items:baseline;gap:12px;padding:3px 0;border-bottom:1px solid #f1f3f8}
  .overview-row .label{flex:0 0 150px;font-size:10px;color:#1d2433;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
  .overview-row .sample{flex:1;min-width:0}
  @media print{.toolbar{display:none} body{padding:0}} @page{margin:12mm}
`;

export async function openReport(data: ReportData) {
  const win = window.open('', '_blank', 'width=900,height=1100');
  if (!win) return;
  const sample = esc(data.sampleText);
  const title = esc(data.title);
  const fam = (f: ReportData['fonts'][number]) => esc(cssFamilyFor(f));
  const body = data.type === 'specimen'
    ? data.fonts.map((f) => `<section class="specimen"><div class="fam"><span class="name">${esc(f.family)}</span>${f.classification ? `${esc(f.classification)} · ` : ''}${f.styleCount} styles</div>
        <div class="sample" style="font-family:${fam(f)};font-size:26px">${sample}</div>
        <div class="sample" style="font-family:${fam(f)};font-size:12px">${sample}</div></section>`).join('')
    : data.fonts.map((f) => `<div class="overview-row"><div class="label">${esc(f.family)}</div>
        <div class="sample" style="font-family:${fam(f)};font-size:16px">${sample}</div></div>`).join('');
  win.document.write(`<!doctype html><html><head><meta charset="utf-8"><title>${title}</title><style>${PRINT_CSS}</style></head>
    <body><div class="toolbar"><h1>${title}</h1><button onclick="window.print()">Save as PDF</button></div>${body}</body></html>`);
  win.document.close();
  const failed: string[] = [];
  await shareFontFaces(data.fonts, win.document, (m) => failed.push(m));
  if (failed.length) {
    console.warn('skrifttype: report fonts failed to load', failed);
    const note = win.document.createElement('div');
    note.className = 'note';
    note.textContent = `${failed.length} font(s) could not be loaded and show a fallback: ${failed.slice(0, 5).join('; ')}`;
    win.document.querySelector('.toolbar')?.append(note);
  }
  await win.document.fonts.ready;
}
