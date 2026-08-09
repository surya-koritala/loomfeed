package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// cspReport is the legacy `application/csp-report` body shape.
type cspReport struct {
	Report struct {
		DocumentURI        string `json:"document-uri"`
		Referrer           string `json:"referrer"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy     string `json:"original-policy"`
		Disposition        string `json:"disposition"`
		BlockedURI         string `json:"blocked-uri"`
		LineNumber         int    `json:"line-number"`
		ColumnNumber       int    `json:"column-number"`
		SourceFile         string `json:"source-file"`
		StatusCode         int    `json:"status-code"`
		ScriptSample       string `json:"script-sample"`
	} `json:"csp-report"`
}

// reportingAPIEntry is the modern `application/reports+json` shape
// (Reporting API v1). Browsers may send either format; we accept both
// and log identically.
type reportingAPIEntry struct {
	Type      string         `json:"type"`
	URL       string         `json:"url"`
	UserAgent string         `json:"user_agent"`
	Body      map[string]any `json:"body"`
}

// CSPReport ingests Content-Security-Policy-Report-Only violation
// reports and logs them at WARN. The endpoint is intentionally simple —
// no rate-limit override, no DB write, no auth. Volume is bounded by
// browser behavior (one report per violation per page load) and the
// global per-IP rate limiter wired up in main.go is sufficient to
// absorb a misbehaving page.
//
// Mounted at POST /api/v1/csp-report and referenced from the frontend's
// `Content-Security-Policy-Report-Only` header via report-uri.
func CSPReport(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	// Cap body to 16KB — a CSP report is small; anything larger is
	// almost certainly garbage or an attempt to flood logs.
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch r.Header.Get("Content-Type") {
	case "application/csp-report", "application/json":
		var report cspReport
		if err := json.Unmarshal(body, &report); err == nil {
			slog.Warn("csp_violation",
				"document_uri", report.Report.DocumentURI,
				"violated_directive", report.Report.ViolatedDirective,
				"effective_directive", report.Report.EffectiveDirective,
				"blocked_uri", report.Report.BlockedURI,
				"source_file", report.Report.SourceFile,
				"line", report.Report.LineNumber,
				"column", report.Report.ColumnNumber,
				"disposition", report.Report.Disposition,
			)
		} else {
			slog.Warn("csp_violation_unparseable", "len", len(body), "err", err.Error())
		}
	case "application/reports+json":
		var entries []reportingAPIEntry
		if err := json.Unmarshal(body, &entries); err == nil {
			for _, e := range entries {
				slog.Warn("csp_violation",
					"reporting_type", e.Type,
					"document_uri", e.URL,
					"user_agent", e.UserAgent,
					"body", e.Body,
				)
			}
		} else {
			slog.Warn("csp_violation_unparseable", "len", len(body), "err", err.Error())
		}
	default:
		// Browsers historically used these content-types interchangeably;
		// log the unknown case so we can adjust if someone adds a new
		// reporting flavor.
		slog.Warn("csp_violation_unknown_content_type", "content_type", r.Header.Get("Content-Type"))
	}

	w.WriteHeader(http.StatusNoContent)
}
