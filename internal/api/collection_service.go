package api

import (
	"context"
	"errors"
	"strings"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/store"
)

// CollectionDTO mirrors Collection in TS. FontIDs are family IDs.
type CollectionDTO struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Color   string  `json:"color"`
	FontIDs []int64 `json:"fontIds"`
	Active  bool    `json:"active"`
}

// CollectionService exposes collection CRUD. Membership is stored per
// face; adding a family adds all of its faces. Changing an active
// collection's membership immediately (de)activates the affected fonts.
type CollectionService struct {
	store *store.Store
	cat   *Catalog
	mgr   *activation.Manager
}

// NewCollectionService wires the service.
func NewCollectionService(st *store.Store, cat *Catalog, mgr *activation.Manager) *CollectionService {
	return &CollectionService{store: st, cat: cat, mgr: mgr}
}

// ListCollections returns every collection with its member families.
func (s *CollectionService) ListCollections() ([]CollectionDTO, error) {
	ctx := context.Background()
	cols, err := s.store.ListCollections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CollectionDTO, 0, len(cols))
	for _, c := range cols {
		dto, err := s.toDTO(ctx, c)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	return out, nil
}

// CreateCollection creates a collection, optionally seeded with families.
func (s *CollectionService) CreateCollection(name, color string, fontIDs []int64) (CollectionDTO, error) {
	ctx := context.Background()
	name = strings.TrimSpace(name)
	if name == "" {
		return CollectionDTO{}, errors.New("collection name is required")
	}
	c, err := s.store.CreateCollection(ctx, name)
	if err != nil {
		return CollectionDTO{}, err
	}
	if color != "" {
		if err := s.store.SetCollectionColor(ctx, c.ID, color); err != nil {
			return CollectionDTO{}, err
		}
		c.Color = color
	}
	if len(fontIDs) > 0 {
		if err := s.AddToCollection(c.ID, fontIDs); err != nil {
			return CollectionDTO{}, err
		}
	}
	return s.toDTO(ctx, c)
}

// RenameCollection changes a collection's name.
func (s *CollectionService) RenameCollection(id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("collection name is required")
	}
	return s.store.RenameCollection(context.Background(), id, name)
}

// SetCollectionColor changes a collection's colour.
func (s *CollectionService) SetCollectionColor(id int64, color string) error {
	return s.store.SetCollectionColor(context.Background(), id, color)
}

// DeleteCollection deactivates (if needed) and removes a collection.
// Fonts it was holding stay active only if another source holds them.
func (s *CollectionService) DeleteCollection(id int64) error {
	ctx := context.Background()
	if _, err := s.mgr.DeactivateCollection(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteCollection(ctx, id)
}

// AddToCollection adds every face of each family.
func (s *CollectionService) AddToCollection(id int64, fontIDs []int64) error {
	ctx := context.Background()
	faces, err := s.cat.FacesOf(ctx, fontIDs...)
	if err != nil {
		return err
	}
	for _, f := range faces {
		if err := s.store.AddFontToCollection(ctx, id, f); err != nil {
			return err
		}
	}
	_, err = s.mgr.SyncCollection(ctx, id)
	return err
}

// RemoveFromCollection removes every face of each family.
func (s *CollectionService) RemoveFromCollection(id int64, fontIDs []int64) error {
	ctx := context.Background()
	faces, err := s.cat.FacesOf(ctx, fontIDs...)
	if err != nil {
		return err
	}
	for _, f := range faces {
		if err := s.store.RemoveFontFromCollection(ctx, id, f); err != nil {
			return err
		}
	}
	_, err = s.mgr.SyncCollection(ctx, id)
	return err
}

func (s *CollectionService) toDTO(ctx context.Context, c store.Collection) (CollectionDTO, error) {
	faces, err := s.store.ListFontsInCollection(ctx, c.ID)
	if err != nil {
		return CollectionDTO{}, err
	}
	fams, err := s.cat.FamiliesOf(ctx, faces)
	if err != nil {
		return CollectionDTO{}, err
	}
	return CollectionDTO{ID: c.ID, Name: c.Name, Color: c.Color, FontIDs: fams, Active: c.Active}, nil
}
