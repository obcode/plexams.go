package plexams

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/rs/zerolog/log"
)

// This file exposes the PDF/CSV/ICS generators as REST downloads for the GUI. Each
// generator builds a maroto document (via a *Maroto builder) which is rendered to bytes
// and streamed; the plexams tool is server-only, so nothing writes to a local file.

// marotoBytes renders a maroto document to PDF bytes.
func marotoBytes(m pdf.Maroto) ([]byte, error) {
	buf, err := m.Output()
	if err != nil {
		log.Error().Err(err).Msg("could not render PDF to bytes")
		return nil, err
	}
	return buf.Bytes(), nil
}

// GenerateExamsToPlanPDFBytes builds the "exams to plan" PDF as bytes.
func (p *Plexams) GenerateExamsToPlanPDFBytes(ctx context.Context) ([]byte, error) {
	m, err := p.generateExamsToPlanMaroto(ctx)
	if err != nil {
		return nil, err
	}
	return marotoBytes(m)
}

// SameModulNamesBytes builds the same-module-name PDF as bytes.
func (p *Plexams) SameModulNamesBytes(ctx context.Context) ([]byte, error) {
	return marotoBytes(p.sameModulNamesMaroto(ctx))
}

// ConstraintsPDFBytes builds the constraints PDF as bytes.
func (p *Plexams) ConstraintsPDFBytes(ctx context.Context) ([]byte, error) {
	m, err := p.constraintsMaroto(ctx)
	if err != nil {
		return nil, err
	}
	return marotoBytes(m)
}

// pdfExport describes one downloadable document, its default download filename and
// content type, and how to build its bytes.
type pdfExport struct {
	filename    string
	contentType string // empty defaults to application/pdf
	build       func(ctx context.Context) ([]byte, error)
}

// pdfExports is the registry of downloadable PDF/document kinds, keyed by the same
// kind strings the old `pdf` CLI command used.
func (p *Plexams) pdfExports() map[string]pdfExport {
	return map[string]pdfExport{
		"exams-to-plan":    {filename: "PrüfungenImPrüfungszeitraum.pdf", build: p.GenerateExamsToPlanPDFBytes},
		"same-module-name": {filename: "PrüfungenMitGleichenModulnamen.pdf", build: p.SameModulNamesBytes},
		"constraints":      {filename: "Constraints.pdf", build: p.ConstraintsPDFBytes},
		"draft-muc.dai":    {filename: "draft-muc.dai.pdf", build: func(ctx context.Context) ([]byte, error) { return marotoBytes(p.draftMucDaiMaroto(ctx)) }},
		"draft-fk08":       {filename: "draft-fk08.pdf", build: func(ctx context.Context) ([]byte, error) { return marotoBytes(p.draftFk08Maroto(ctx)) }},
		"draft-fk10":       {filename: "draft-fk10.pdf", build: func(ctx context.Context) ([]byte, error) { return marotoBytes(p.draftFk10Maroto(ctx)) }},
		"draft-exahm":      {filename: "draft-exahm.pdf", build: func(ctx context.Context) ([]byte, error) { return marotoBytes(p.draftExahmMaroto(ctx)) }},
		"draft-fs":         {filename: "draft-fs.pdf", build: p.DraftFSBytes},
		"draft-lba-rep":    {filename: "draft-lba-rep.pdf", build: p.DraftLbaRepBytes},
		"draft-si":         {filename: "draft-si.zip", contentType: "application/zip", build: p.DraftSIZipBytes},
	}
}

// PDFExportKinds returns the known PDF export kinds in a stable order.
func (p *Plexams) PDFExportKinds() []string {
	exports := p.pdfExports()
	kinds := make([]string, 0, len(exports))
	for k := range exports {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// HTTPDownloadPDF streams one generated PDF (or the draft-si ZIP) as a download.
// GET /download/pdf/{kind}
func (p *Plexams) HTTPDownloadPDF(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	export, ok := p.pdfExports()[kind]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown pdf kind %q (known: %s)", kind, strings.Join(p.PDFExportKinds(), ", ")), http.StatusBadRequest)
		return
	}
	data, err := export.build(r.Context())
	if err != nil {
		http.Error(w, "cannot generate pdf: "+err.Error(), http.StatusInternalServerError)
		return
	}
	contentType := export.contentType
	if contentType == "" {
		contentType = "application/pdf"
	}
	filename := fmt.Sprintf("%s_%s", strings.ReplaceAll(p.semester, " ", "_"), export.filename)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Str("kind", kind).Msg("cannot write pdf download")
	}
}
