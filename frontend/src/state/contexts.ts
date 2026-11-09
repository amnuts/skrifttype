import { createContext } from '@lit/context';
import type { PreferencesStore } from './preferences-store';
import type { LibraryStore } from './library-store';
import type { ActivationStore } from './activation-store';
import type { CollectionsStore } from './collections-store';

export const preferencesContext = createContext<PreferencesStore>('preferences');
export const libraryContext = createContext<LibraryStore>('library');
export const activationContext = createContext<ActivationStore>('activation');
export const collectionsContext = createContext<CollectionsStore>('collections');
