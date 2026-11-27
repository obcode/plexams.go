package plexams

import (
	"io"
	"net/http"
	"strings"
)

// HTTPUploadNTAList reads one or more NTA lists (multipart field "file", repeated)
// and stores each as the current list of its faculty. The NTA master data is not
// touched; the GUI shows the comparison (ntaListPreview) and applies what the
// planner selects (applyNTAList).
func (p *Plexams) HTTPUploadNTAList(w http.ResponseWriter, r *http.Request) {
	if !p.WritesAllowed() {
		http.Error(w, "Gerade läuft eine Validierung, ein Import, eine Übertragung oder ein E-Mail-Versand; der Upload geht erst danach.", http.StatusConflict)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Der Upload konnte nicht gelesen werden; bitte die Datei noch einmal auswählen. ("+err.Error()+")", http.StatusBadRequest)
		return
	}
	headers := r.MultipartForm.File["file"]
	if len(headers) == 0 {
		http.Error(w, "Es wurde keine Datei mitgeschickt; bitte die NTA-Liste(n) aus dem SharePoint-Ordner auswählen.", http.StatusBadRequest)
		return
	}
	files := make([]NTAListFile, 0, len(headers))
	names := make([]string, 0, len(headers))
	for _, h := range headers {
		f, err := h.Open()
		if err != nil {
			http.Error(w, h.Filename+" konnte nicht gelesen werden.", http.StatusBadRequest)
			return
		}
		data, err := io.ReadAll(f)
		f.Close() //nolint:errcheck
		if err != nil {
			http.Error(w, h.Filename+" konnte nicht gelesen werden.", http.StatusBadRequest)
			return
		}
		files = append(files, NTAListFile{Filename: h.Filename, Data: data})
		names = append(names, h.Filename)
	}

	ctx := r.Context()
	summaries, err := p.UploadNTALists(ctx, files)
	if err != nil {
		http.Error(w, "Die NTA-Liste wurde nicht eingelesen. "+err.Error(), http.StatusBadRequest)
		return
	}
	p.LogUpload(ctx, "uploadNTAList", "files", strings.Join(names, ", "))
	writeJSON(w, map[string]any{"lists": summaries})
}
