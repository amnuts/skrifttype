// Drag-and-drop payload for moving font families onto a collection.
export const FONT_DRAG_MIME = 'application/x-skrifttype-fonts';

export function setDraggedFonts(dt: DataTransfer, ids: number[]) {
  dt.setData(FONT_DRAG_MIME, JSON.stringify(ids));
  dt.effectAllowed = 'copy';
}

export function isFontDrag(dt: DataTransfer | null): boolean {
  return !!dt && Array.from(dt.types).includes(FONT_DRAG_MIME);
}

export function draggedFonts(dt: DataTransfer | null): number[] {
  try {
    const ids = JSON.parse(dt?.getData(FONT_DRAG_MIME) || '[]');
    return Array.isArray(ids) ? ids.filter((n) => Number.isInteger(n)) : [];
  } catch { return []; }
}
