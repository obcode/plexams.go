package plexams

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/obcode/plexams.go/db"
	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

const (
	// AttachmentKindCoverPage holds per-teacher cover-page PDFs (key = teacher ID).
	AttachmentKindCoverPage = "cover-page"
	// AttachmentKindInvigilationImage holds per-invigilator PNGs (key = invigilator ID).
	AttachmentKindInvigilationImage = "invigilation-image"
)

// trailingDigits extracts the trailing integer of a base filename (without
// extension), e.g. "cover-12345.pdf" -> "12345". Used to derive the attachment
// key when bulk-importing a ZIP.
var trailingDigits = regexp.MustCompile(`(\d+)$`)

// SaveEmailAttachment stores one uploaded attachment for later sending.
func (p *Plexams) SaveEmailAttachment(ctx context.Context, kind, key, filename, contentType string, data []byte) error {
	return p.dbClient.SaveEmailAttachment(ctx, &db.EmailAttachment{
		Kind:        kind,
		Key:         key,
		Filename:    filename,
		ContentType: contentType,
		Size:        len(data),
		Data:        data,
		UploadedAt:  time.Now(),
	})
}

// EmailAttachmentInfos lists the uploaded attachments of a kind (without data).
func (p *Plexams) EmailAttachmentInfos(ctx context.Context, kind string) ([]*model.EmailAttachmentInfo, error) {
	atts, err := p.dbClient.EmailAttachmentInfos(ctx, kind)
	if err != nil {
		return nil, err
	}
	infos := make([]*model.EmailAttachmentInfo, 0, len(atts))
	for _, a := range atts {
		infos = append(infos, &model.EmailAttachmentInfo{
			Kind:        a.Kind,
			Key:         a.Key,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			UploadedAt:  a.UploadedAt,
		})
	}
	return infos, nil
}

// ClearEmailAttachments removes all uploaded attachments of a kind.
func (p *Plexams) ClearEmailAttachments(ctx context.Context, kind string) (int, error) {
	return p.dbClient.ClearEmailAttachments(ctx, kind)
}

// GetEmailAttachment returns one stored attachment incl. its data (or nil).
func (p *Plexams) GetEmailAttachment(ctx context.Context, kind, key string) (*db.EmailAttachment, error) {
	return p.dbClient.GetEmailAttachment(ctx, kind, key)
}

// contentTypeFor returns the given content type, or guesses one from the
// filename extension, falling back to application/octet-stream.
func contentTypeFor(given, filename string) string {
	if given != "" && given != "application/octet-stream" {
		return given
	}
	if ct := mime.TypeByExtension(filepath.Ext(filename)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// HTTPUploadEmailAttachment handles a single-file upload:
// POST /upload/email-attachment  (multipart form: kind, key, file).
func (p *Plexams) HTTPUploadEmailAttachment(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "a validation or transfer/email is running, cannot upload now", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	key := r.FormValue("key")
	if kind == "" || key == "" {
		http.Error(w, "kind and key are required", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ct := contentTypeFor(header.Header.Get("Content-Type"), header.Filename)
	if err := p.SaveEmailAttachment(r.Context(), kind, key, header.Filename, ct, data); err != nil {
		log.Error().Err(err).Str("kind", kind).Str("key", key).Msg("cannot save email attachment")
		http.Error(w, "cannot store attachment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"stored": 1, "kind": kind, "key": key, "filename": header.Filename, "size": len(data)})
}

// HTTPUploadEmailAttachmentsZip handles a ZIP bulk upload:
// POST /upload/email-attachments-zip  (multipart form: kind, file=<zip>).
// The key of each entry is the trailing integer of its filename
// (e.g. "<prefix>12345.pdf" -> "12345").
func (p *Plexams) HTTPUploadEmailAttachmentsZip(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "a validation or transfer/email is running, cannot upload now", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		http.Error(w, "cannot parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	if kind == "" {
		http.Error(w, "kind is required", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close() // nolint:errcheck

	zipData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "cannot read zip: "+err.Error(), http.StatusInternalServerError)
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		http.Error(w, "not a valid zip: "+err.Error(), http.StatusBadRequest)
		return
	}

	stored := 0
	skipped := make([]string, 0)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "__MACOSX") {
			continue
		}
		name := strings.TrimSuffix(base, filepath.Ext(base))
		m := trailingDigits.FindStringSubmatch(name)
		if m == nil {
			skipped = append(skipped, base+" (no key in filename)")
			continue
		}
		key := m[1]

		rc, err := f.Open()
		if err != nil {
			skipped = append(skipped, base+" (cannot open)")
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close() // nolint:errcheck
		if err != nil {
			skipped = append(skipped, base+" (cannot read)")
			continue
		}

		ct := contentTypeFor("", base)
		if err := p.SaveEmailAttachment(r.Context(), kind, key, base, ct, data); err != nil {
			log.Error().Err(err).Str("kind", kind).Str("key", key).Msg("cannot save email attachment from zip")
			skipped = append(skipped, base+" (cannot store)")
			continue
		}
		stored++
	}

	writeJSON(w, map[string]any{"stored": stored, "skipped": skipped, "kind": kind})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("cannot write json response")
	}
}
