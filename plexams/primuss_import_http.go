package plexams

import (
	"context"
	"io"
	"net/http"
	"sort"

	"github.com/obcode/plexams.go/plexams/primuss"
	"github.com/rs/zerolog/log"
)

// HTTPUploadPrimussZip imports the Primuss Sammellisten XLSX from an uploaded ZIP
// (multipart form field "file"). It replaces the per-program studentregs_/exams_/count_/
// conflicts_ collections (only for the programs present in the ZIP) and returns a JSON
// summary including the ZPA ancodes whose Primuss data changed (so the GUI can offer
// update emails).
func (p *Plexams) HTTPUploadPrimussZip(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "Gerade läuft eine Validierung, ein Import, eine Übertragung oder ein E-Mail-Versand; der Upload geht erst danach.", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "Der Upload konnte nicht gelesen werden; bitte die Datei noch einmal auswählen. ("+err.Error()+")", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Es wurde keine Datei mitgeschickt; bitte das ZIP mit den Sammellisten auswählen. ("+err.Error()+")", http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	zipData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Die hochgeladene Datei konnte nicht gelesen werden. ("+err.Error()+")", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	result, err := p.ImportPrimussZip(ctx, zipData)
	if err != nil {
		http.Error(w, "Der Import ist fehlgeschlagen; es wurde nichts übernommen. "+err.Error(), http.StatusBadRequest)
		return
	}

	p.markCondition(ctx, condPrimussImported)
	p.LogUpload(ctx, "uploadPrimussZip", "file", header.Filename)

	programs := make([]map[string]any, 0, len(result.Programs))
	for _, pr := range result.Programs {
		programs = append(programs, map[string]any{
			"program":        pr.Program,
			"exams":          pr.ExamsImported,
			"studentRegs":    pr.StudentRegs,
			"count":          pr.CountRows,
			"conflicts":      pr.ConflictRows,
			"missing":        pr.Missing,
			"firstImport":    pr.FirstImport,
			"changedAncodes": pr.ChangedAncodes,
		})
	}

	writeJSON(w, map[string]any{
		"programs":           programs,
		"skipped":            result.Skipped,
		"affectedZpaAncodes": p.affectedZpaAncodes(ctx, result),
	})
}

// affectedZpaAncodes maps the changed Primuss (program, ancode) of an import to the ZPA
// ancodes that carry them, so the GUI can send a Primuss-data update email per ZPA exam.
func (p *Plexams) affectedZpaAncodes(ctx context.Context, result *primuss.ImportResult) []int {
	zpaByPrimuss, err := p.zpaExamsByPrimussAncode(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot map changed primuss ancodes to zpa exams")
		return []int{}
	}
	set := make(map[int]bool)
	for _, pr := range result.Programs {
		for _, ancode := range pr.ChangedAncodes {
			for _, zpaAncode := range zpaByPrimuss[primussKey{pr.Program, ancode}] {
				set[zpaAncode] = true
			}
		}
	}
	out := make([]int, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Ints(out)
	return out
}
