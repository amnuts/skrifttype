import './components/app-shell';

// Apply persisted theme before first paint to avoid a flash.
const saved = localStorage.getItem('skrifttype.theme');
const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
const theme = saved ?? (prefersDark ? 'dark' : 'light');
document.documentElement.setAttribute('data-theme', theme === 'system' ? (prefersDark ? 'dark' : 'light') : theme);
