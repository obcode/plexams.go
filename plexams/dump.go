package plexams

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

// dumpFormatVersion is the on-disk format version of the semester/dataset dumps.
// Bump it on an incompatible change to the file layout.
const dumpFormatVersion = 1

// ErrDatabaseNotEmpty is returned by RestoreSemesterDump when the target database
// already holds planning data (a full-semester dump may only be restored into a
// fresh/empty workspace, never on top of an existing semester).
var ErrDatabaseNotEmpty = errors.New("database is not empty")

// dumpBookkeepingCollections are collections that a freshly created (empty)
// workspace already carries; they don't count towards the emptiness check of a
// semester restore and are never overwritten by it.
var dumpBookkeepingCollections = map[string]bool{
	"semester_config_input": true,
	"semester_config":       true,
	"semester_meta":         true,
	"mutation_log":          true,
	"sync_log":              true,
}

// collectionDump is the envelope for one collection's documents in a semester ZIP.
// Extended JSON cannot be a top-level array, so the documents are wrapped here.
type collectionDump struct {
	Documents []bson.M `bson:"documents"`
}

// semesterDumpManifest describes a whole-semester dump (one entry in the ZIP).
type semesterDumpManifest struct {
	Semester   string         `json:"semester"`
	Database   string         `json:"database"`
	Format     int            `json:"format"`
	ExportedAt time.Time      `json:"exportedAt"`
	Counts     map[string]int `json:"counts"`
}

// RestoreResult reports how many documents were written per collection.
type RestoreResult struct {
	Restored map[string]int `json:"restored"`
	Total    int            `json:"total"`
}

func newRestoreResult() *RestoreResult {
	return &RestoreResult{Restored: make(map[string]int)}
}

func (r *RestoreResult) add(name string, n int) {
	r.Restored[name] = n
	r.Total += n
}

// SemesterDumpZip builds an in-memory ZIP with one MongoDB-Extended-JSON file per
// collection of the current (per-semester) database, plus a manifest.json. It is a
// full clone: imported ZPA/Primuss data is included alongside the local overlay, so
// the semester can be restored without re-importing. Types (ObjectIDs, dates, ints)
// are preserved via extended JSON.
func (p *Plexams) SemesterDumpZip(ctx context.Context) ([]byte, error) {
	names, err := p.dbClient.AllCollectionNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot list collections: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	counts := make(map[string]int, len(names))

	for _, name := range names {
		docs, err := p.dbClient.RawCollection(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("cannot read collection %q: %w", name, err)
		}
		data, err := bson.MarshalExtJSON(collectionDump{Documents: docs}, true, false)
		if err != nil {
			return nil, fmt.Errorf("cannot encode collection %q: %w", name, err)
		}
		f, err := zw.Create(name + ".json")
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
		counts[name] = len(docs)
	}

	manifest := semesterDumpManifest{
		Semester:   p.semester,
		Database:   p.dbClient.DatabaseName(),
		Format:     dumpFormatVersion,
		ExportedAt: time.Now(),
		Counts:     counts,
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	mf, err := zw.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err := mf.Write(mb); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RestoreSemesterDump restores a whole-semester ZIP (as produced by SemesterDumpZip)
// into the current database. It refuses (ErrDatabaseNotEmpty) if the database already
// holds planning data, so an existing semester can never be clobbered — create/switch
// to a fresh workspace first. The per-database semester_meta is intentionally left
// untouched so the workspace keeps its own identity and read-only flag.
func (p *Plexams) RestoreSemesterDump(ctx context.Context, zipData []byte) (*RestoreResult, error) {
	names, err := p.dbClient.AllCollectionNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot list collections: %w", err)
	}
	for _, name := range names {
		if dumpBookkeepingCollections[name] {
			continue
		}
		count, err := p.dbClient.CountCollection(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("cannot count collection %q: %w", name, err)
		}
		if count > 0 {
			return nil, fmt.Errorf("%w: %q contains %d documents (collection %q)",
				ErrDatabaseNotEmpty, p.dbClient.DatabaseName(), count, name)
		}
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}

	result := newRestoreResult()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(zf.Name)
		if base == "manifest.json" || !strings.HasSuffix(base, ".json") {
			continue
		}
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "__MACOSX") {
			continue
		}
		coll := strings.TrimSuffix(base, ".json")
		// keep the target database's own identity/read-only flag
		if coll == "semester_meta" {
			continue
		}

		docs, err := readExtJSONDocs(zf)
		if err != nil {
			return nil, fmt.Errorf("cannot decode %q: %w", base, err)
		}
		n, err := p.dbClient.ReplaceRawCollection(ctx, coll, docs)
		if err != nil {
			return nil, fmt.Errorf("cannot restore collection %q: %w", coll, err)
		}
		result.add(coll, n)
	}

	log.Info().Str("database", p.dbClient.DatabaseName()).Int("documents", result.Total).
		Msg("restored semester dump")
	return result, nil
}

func readExtJSONDocs(zf *zip.File) ([]bson.M, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var envelope collectionDump
	if err := bson.UnmarshalExtJSON(data, true, &envelope); err != nil {
		return nil, err
	}
	if envelope.Documents == nil {
		return []bson.M{}, nil
	}
	return envelope.Documents, nil
}

// ---- per-page datasets --------------------------------------------------------

// datasetSpec maps a logical dataset (a GUI page's data) to the collection(s) it
// downloads/restores. A simple dataset fully replaces its collections; the external
// dataset additionally carries the exam times, which live in the shared plan
// collection and are therefore merged by ancode instead of replaced wholesale.
type datasetSpec struct {
	Title       string
	Collections []string
	external    bool
}

// datasetRegistry is the allow-list of datasets exposed for per-page download/upload.
// Collection name strings mirror the (unexported) constants in the db package.
var datasetRegistry = map[string]datasetSpec{
	"constraints":    {Title: "Constraints (inkl. notPlannedByMe)", Collections: []string{"constraints"}},
	"external-exams": {Title: "Externe Prüfungen + Zeiten", Collections: []string{"non_zpaexams", "plan"}, external: true},
	"preplan":        {Title: "Vorplanung (SEB/EXaHM)", Collections: []string{"preplan_exams"}},
	"mucdai-links":   {Title: "MUC.DAI-Verknüpfungen", Collections: []string{"mucdai_links"}},
	"room-requests":  {Title: "Raumanfragen", Collections: []string{"room_requests"}},
}

// DatasetNames returns the sorted list of known dataset keys.
func DatasetNames() []string {
	names := make([]string, 0, len(datasetRegistry))
	for k := range datasetRegistry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// datasetManifest describes a single-dataset dump.
type datasetManifest struct {
	Dataset    string         `bson:"dataset" json:"dataset"`
	Semester   string         `bson:"semester" json:"semester"`
	Format     int            `bson:"format" json:"format"`
	ExportedAt time.Time      `bson:"exportedAt" json:"exportedAt"`
	Counts     map[string]int `bson:"counts" json:"counts"`
}

// datasetDump is the (extended-JSON) file format of a per-page dataset export.
type datasetDump struct {
	Manifest    datasetManifest     `bson:"manifest" json:"manifest"`
	Collections map[string][]bson.M `bson:"collections" json:"collections"`
}

// DatasetDumpJSON builds the extended-JSON export of a single dataset and a
// suggested download filename.
func (p *Plexams) DatasetDumpJSON(ctx context.Context, name string) ([]byte, string, error) {
	spec, ok := datasetRegistry[name]
	if !ok {
		return nil, "", fmt.Errorf("unknown dataset %q", name)
	}

	dump := datasetDump{
		Manifest: datasetManifest{
			Dataset:    name,
			Semester:   p.semester,
			Format:     dumpFormatVersion,
			ExportedAt: time.Now(),
			Counts:     make(map[string]int),
		},
		Collections: make(map[string][]bson.M),
	}

	if spec.external {
		ext, err := p.dbClient.RawCollection(ctx, "non_zpaexams")
		if err != nil {
			return nil, "", err
		}
		dump.Collections["non_zpaexams"] = ext
		dump.Manifest.Counts["non_zpaexams"] = len(ext)

		allPlan, err := p.dbClient.RawCollection(ctx, "plan")
		if err != nil {
			return nil, "", err
		}
		extPlan := filterByAncode(allPlan, ancodeSet(ext))
		dump.Collections["plan"] = extPlan
		dump.Manifest.Counts["plan"] = len(extPlan)
	} else {
		for _, coll := range spec.Collections {
			docs, err := p.dbClient.RawCollection(ctx, coll)
			if err != nil {
				return nil, "", err
			}
			dump.Collections[coll] = docs
			dump.Manifest.Counts[coll] = len(docs)
		}
	}

	data, err := bson.MarshalExtJSON(&dump, true, false)
	if err != nil {
		return nil, "", err
	}
	// name the file after the physical database, not the logical semester, so a
	// workspace clone (e.g. Test26SS with logical semester 2026-SS) is unambiguous.
	filename := fmt.Sprintf("%s_%s.json", strings.ReplaceAll(p.dbClient.DatabaseName(), " ", "_"), name)
	return data, filename, nil
}

// RestoreDataset restores a single dataset export into the current database. Simple
// datasets replace their collections; the external dataset replaces non_zpaexams and
// re-inserts only the external plan entries (by ancode), leaving the rest of the
// schedule untouched.
func (p *Plexams) RestoreDataset(ctx context.Context, name string, data []byte) (*RestoreResult, error) {
	spec, ok := datasetRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown dataset %q", name)
	}

	var dump datasetDump
	if err := bson.UnmarshalExtJSON(data, true, &dump); err != nil {
		return nil, fmt.Errorf("file is not a valid dataset export: %w", err)
	}
	if dump.Manifest.Dataset != "" && dump.Manifest.Dataset != name {
		return nil, fmt.Errorf("file holds dataset %q, but %q was expected", dump.Manifest.Dataset, name)
	}

	result := newRestoreResult()

	if spec.external {
		incoming := dump.Collections["non_zpaexams"]
		planDocs := dump.Collections["plan"]

		// clear the external plan entries before re-inserting them, so no stale
		// external time survives; never touch the regular (non-external) schedule.
		current, err := p.dbClient.RawCollection(ctx, "non_zpaexams")
		if err != nil {
			return nil, err
		}
		clear := unionAncodes(ancodeSet(current), ancodeSet(incoming), ancodeSet(planDocs))
		if _, err := p.dbClient.DeleteDocsByAncodes(ctx, "plan", clear); err != nil {
			return nil, err
		}
		n, err := p.dbClient.ReplaceRawCollection(ctx, "non_zpaexams", incoming)
		if err != nil {
			return nil, err
		}
		result.add("non_zpaexams", n)
		m, err := p.dbClient.InsertRawDocs(ctx, "plan", planDocs)
		if err != nil {
			return nil, err
		}
		result.add("plan (Zeiten)", m)
	} else {
		for _, coll := range spec.Collections {
			n, err := p.dbClient.ReplaceRawCollection(ctx, coll, dump.Collections[coll])
			if err != nil {
				return nil, fmt.Errorf("cannot restore collection %q: %w", coll, err)
			}
			result.add(coll, n)
		}
	}

	log.Info().Str("dataset", name).Int("documents", result.Total).Msg("restored dataset")
	return result, nil
}

func ancodeSet(docs []bson.M) map[int]bool {
	s := make(map[int]bool, len(docs))
	for _, d := range docs {
		if a, ok := toInt(d["ancode"]); ok {
			s[a] = true
		}
	}
	return s
}

func filterByAncode(docs []bson.M, ancodes map[int]bool) []bson.M {
	out := make([]bson.M, 0)
	for _, d := range docs {
		if a, ok := toInt(d["ancode"]); ok && ancodes[a] {
			out = append(out, d)
		}
	}
	return out
}

func unionAncodes(sets ...map[int]bool) []int {
	seen := make(map[int]bool)
	out := make([]int, 0)
	for _, s := range sets {
		for a := range s {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

// ---- CLI file helpers ---------------------------------------------------------

// ExportSemesterDump writes the whole-semester ZIP to a file (CLI).
func (p *Plexams) ExportSemesterDump(zipfile string) error {
	data, err := p.SemesterDumpZip(context.Background())
	if err != nil {
		return err
	}
	if err := os.WriteFile(zipfile, data, 0644); err != nil {
		log.Error().Err(err).Str("file", zipfile).Msg("cannot write semester dump")
		return err
	}
	return nil
}

// ImportSemesterDump restores a whole-semester ZIP file into the current database (CLI).
func (p *Plexams) ImportSemesterDump(zipfile string) (*RestoreResult, error) {
	data, err := os.ReadFile(zipfile)
	if err != nil {
		return nil, err
	}
	return p.RestoreSemesterDump(context.Background(), data)
}

// ExportDataset writes a single per-page dataset to a JSON file (CLI).
func (p *Plexams) ExportDataset(name, jsonfile string) error {
	data, _, err := p.DatasetDumpJSON(context.Background(), name)
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonfile, data, 0644); err != nil {
		log.Error().Err(err).Str("file", jsonfile).Msg("cannot write dataset dump")
		return err
	}
	return nil
}

// ImportDataset restores a single per-page dataset JSON file into the current database (CLI).
func (p *Plexams) ImportDataset(name, jsonfile string) (*RestoreResult, error) {
	data, err := os.ReadFile(jsonfile)
	if err != nil {
		return nil, err
	}
	return p.RestoreDataset(context.Background(), name, data)
}

// ---- HTTP handlers ------------------------------------------------------------

// HTTPDownloadSemesterDump streams the current semester as a ZIP download.
// GET /download/semester-dump.zip
func (p *Plexams) HTTPDownloadSemesterDump(w http.ResponseWriter, r *http.Request) {
	data, err := p.SemesterDumpZip(r.Context())
	if err != nil {
		http.Error(w, "cannot build semester dump: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// name the file after the physical database, not the logical semester, so a
	// workspace clone (e.g. Test26SS with logical semester 2026-SS) is unambiguous.
	filename := fmt.Sprintf("%s_semester-dump.zip", strings.ReplaceAll(p.dbClient.DatabaseName(), " ", "_"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Msg("cannot write semester dump download")
	}
}

// HTTPUploadSemesterDump restores a semester ZIP into the current (fresh) database.
// POST /upload/semester-dump.zip  (multipart form: file=<zip>)
func (p *Plexams) HTTPUploadSemesterDump(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "a validation or transfer/email is running, cannot upload now", http.StatusConflict)
		return
	}
	if p.IsReadOnly() {
		http.Error(w, "semester is read-only", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(1 << 30); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	zipData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read zip: "+err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := p.RestoreSemesterDump(r.Context(), zipData)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrDatabaseNotEmpty) {
			status = http.StatusConflict
		}
		http.Error(w, "restore failed: "+err.Error(), status)
		return
	}
	writeJSON(w, result)
}

// HTTPDownloadDataset streams one per-page dataset as a JSON download.
// GET /download/dataset?name=<dataset>
func (p *Plexams) HTTPDownloadDataset(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	data, filename, err := p.DatasetDumpJSON(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Str("dataset", name).Msg("cannot write dataset download")
	}
}

// HTTPUploadDataset restores one per-page dataset into the current database.
// POST /upload/dataset  (multipart form: name=<dataset>, file=<json>)
func (p *Plexams) HTTPUploadDataset(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "a validation or transfer/email is running, cannot upload now", http.StatusConflict)
		return
	}
	if p.IsReadOnly() {
		http.Error(w, "semester is read-only", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := p.RestoreDataset(r.Context(), name, data)
	if err != nil {
		http.Error(w, "restore failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, result)
}
