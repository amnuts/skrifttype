package api

import (
	"context"
	"fmt"
	"slices"

	"github.com/amnuts/skrifttype/internal/activation"
	"github.com/amnuts/skrifttype/internal/store"
)

// FontErrorDTO reports a family that could not be activated.
type FontErrorDTO struct {
	FontID int64  `json:"fontId"`
	Error  string `json:"error"`
}

// ActivationDiffDTO mirrors ActivationDiff in TS, in family IDs.
type ActivationDiffDTO struct {
	Loaded   []int64        `json:"loaded"`
	Unloaded []int64        `json:"unloaded"`
	NoOp     []int64        `json:"noop"`
	Failed   []FontErrorDTO `json:"failed"`
}

// ActivationService exposes on-demand activation to the frontend.
// Every method takes and returns family IDs; the catalogue expands
// them to faces for the activation manager.
type ActivationService struct {
	store *store.Store
	cat   *Catalog
	mgr   *activation.Manager
	emit  Emitter
}

// ActivationProgressDTO reports how far a bulk activation has got.
type ActivationProgressDTO struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// bulkChunk is how many families a bulk operation applies per step, so the
// UI can follow along.
const bulkChunk = 25

// NewActivationService wires the service.
func NewActivationService(st *store.Store, cat *Catalog, mgr *activation.Manager, emit Emitter) *ActivationService {
	if emit == nil {
		emit = func(string, any) {}
	}
	return &ActivationService{store: st, cat: cat, mgr: mgr, emit: emit}
}

// GetActiveSet returns, per active family, the source keys holding it
// active: "manual" and/or "collection:<id>".
func (s *ActivationService) GetActiveSet() (map[int64][]string, error) {
	ctx := context.Background()
	rows, err := s.store.ListActivations(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := s.cat.index(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64][]string)
	for _, r := range rows {
		fam, ok := idx.faceFam[r.FontID]
		if !ok {
			continue
		}
		key := sourceKey(r.Source)
		if !slices.Contains(out[fam], key) {
			out[fam] = append(out[fam], key)
		}
	}
	return out, nil
}

func sourceKey(src store.ActivationSource) string {
	if src.Kind == store.ActivationKindCollection && src.SourceID.Valid {
		return fmt.Sprintf("collection:%d", src.SourceID.Int64)
	}
	return src.Kind
}

// ActivateManual activates every face of the family.
func (s *ActivationService) ActivateManual(id int64) (ActivationDiffDTO, error) {
	ctx := context.Background()
	faces, err := s.cat.FacesOf(ctx, id)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	d, err := s.mgr.ActivateManual(ctx, faces...)
	return s.toDTO(ctx, d, err)
}

// DeactivateManual drops the manual source from every face of the
// family. Faces also held by an active collection stay loaded.
func (s *ActivationService) DeactivateManual(id int64) (ActivationDiffDTO, error) {
	ctx := context.Background()
	faces, err := s.cat.FacesOf(ctx, id)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	d, err := s.mgr.DeactivateManual(ctx, faces...)
	return s.toDTO(ctx, d, err)
}

// Deactivate turns the family off whatever is holding it: the manual
// source and any active collection. Re-toggling the collection restores it.
func (s *ActivationService) Deactivate(id int64) (ActivationDiffDTO, error) {
	ctx := context.Background()
	faces, err := s.cat.FacesOf(ctx, id)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	d, err := s.mgr.Deactivate(ctx, faces...)
	return s.toDTO(ctx, d, err)
}

// ActivateAllAvailable switches on every activatable font in the library
// (System fonts and web fonts are skipped).
func (s *ActivationService) ActivateAllAvailable() (ActivationDiffDTO, error) {
	ctx := context.Background()
	idx, err := s.cat.index(ctx)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	var groups [][]int64 // one entry per family
	for _, fam := range idx.families {
		var faces []int64
		for _, st := range fam.dto.Styles {
			if st.Activatable && st.Available {
				faces = append(faces, st.ID)
			}
		}
		if len(faces) > 0 {
			groups = append(groups, faces)
		}
	}
	return s.inChunks(ctx, groups, s.mgr.ActivateManual)
}

// DeactivateAll switches off every active font, whatever is holding it.
func (s *ActivationService) DeactivateAll() (ActivationDiffDTO, error) {
	ctx := context.Background()
	ids, err := s.store.ListActiveFontIDs(ctx)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	idx, err := s.cat.index(ctx)
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	byFamily := map[int64][]int64{}
	var order []int64
	for _, id := range ids {
		fam, ok := idx.faceFam[id]
		if !ok {
			fam = -id // not in the catalogue (e.g. its folder is disabled): still switch it off
		}
		if _, seen := byFamily[fam]; !seen {
			order = append(order, fam)
		}
		byFamily[fam] = append(byFamily[fam], id)
	}
	groups := make([][]int64, 0, len(order))
	for _, fam := range order {
		groups = append(groups, byFamily[fam])
	}
	return s.inChunks(ctx, groups, s.mgr.Deactivate)
}

// inChunks applies fn to the font groups (one per family) a chunk at a
// time, emitting progress in families after each, and returns the combined
// diff.
func (s *ActivationService) inChunks(ctx context.Context, groups [][]int64,
	fn func(context.Context, ...int64) (activation.Diff, error)) (ActivationDiffDTO, error) {
	var all activation.Diff
	s.emit(EventActivationProgress, ActivationProgressDTO{Done: 0, Total: len(groups)})
	for start := 0; start < len(groups); start += bulkChunk {
		end := min(start+bulkChunk, len(groups))
		var ids []int64
		for _, g := range groups[start:end] {
			ids = append(ids, g...)
		}
		d, err := fn(ctx, ids...)
		all.Loaded = append(all.Loaded, d.Loaded...)
		all.Unloaded = append(all.Unloaded, d.Unloaded...)
		all.NoOp = append(all.NoOp, d.NoOp...)
		all.Failed = append(all.Failed, d.Failed...)
		if err != nil {
			return s.toDTO(ctx, all, err)
		}
		s.emit(EventActivationProgress, ActivationProgressDTO{Done: end, Total: len(groups)})
	}
	return s.toDTO(ctx, all, nil)
}

// ActivateCollection switches a collection on.
func (s *ActivationService) ActivateCollection(id int64) (ActivationDiffDTO, error) {
	ctx := context.Background()
	d, err := s.mgr.ActivateCollection(ctx, id)
	return s.toDTO(ctx, d, err)
}

// DeactivateCollection switches a collection off.
func (s *ActivationService) DeactivateCollection(id int64) (ActivationDiffDTO, error) {
	ctx := context.Background()
	d, err := s.mgr.DeactivateCollection(ctx, id)
	return s.toDTO(ctx, d, err)
}

// Rebroadcast re-sends WM_FONTCHANGE for apps that missed it.
func (s *ActivationService) Rebroadcast() error { return s.mgr.Rebroadcast() }

func (s *ActivationService) toDTO(ctx context.Context, d activation.Diff, err error) (ActivationDiffDTO, error) {
	if err != nil {
		return ActivationDiffDTO{}, err
	}
	idx, ierr := s.cat.index(ctx)
	if ierr != nil {
		return ActivationDiffDTO{}, ierr
	}
	out := ActivationDiffDTO{
		Loaded:   idx.familiesOf(d.Loaded),
		Unloaded: idx.familiesOf(d.Unloaded),
		NoOp:     idx.familiesOf(d.NoOp),
		Failed:   []FontErrorDTO{},
	}
	seen := map[int64]bool{}
	for _, f := range d.Failed {
		fam, ok := idx.faceFam[f.FontID]
		if !ok || seen[fam] {
			continue
		}
		seen[fam] = true
		out.Failed = append(out.Failed, FontErrorDTO{FontID: fam, Error: f.Err})
	}
	return out, nil
}
