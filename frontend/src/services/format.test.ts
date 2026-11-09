import { describe, expect, it } from 'vitest';
import { styleLabels } from './format';

const s = (styleName: string, path: string) => ({ styleName, path });

describe('styleLabels', () => {
  it('leaves unique style names alone and defaults empty ones to Regular', () => {
    expect(styleLabels([s('Bold', 'C:/f/a-Bold.ttf'), s('', 'C:/f/a.ttf')])).toEqual(['Bold', 'Regular']);
  });

  it('tells Nerd Font Mono and proportional builds apart', () => {
    const dir = 'C:/Fonts/JetBrains Mono Extra Bold Nerd Font Complete';
    expect(styleLabels([
      s('Extra Bold', `${dir} Windows Compatible.ttf`),
      s('Extra Bold', `${dir} Mono Windows Compatible.ttf`),
      s('Bold', 'C:/Fonts/JetBrains Mono Bold Nerd Font Complete.ttf'),
    ])).toEqual(['Extra Bold', 'Extra Bold · Mono', 'Bold']);
  });

  it('numbers files whose names give no difference', () => {
    expect(styleLabels([s('Regular', 'C:/a/x.ttf'), s('Regular', 'C:/b/x.ttf')]))
      .toEqual(['Regular · 1', 'Regular · 2']);
  });
});
