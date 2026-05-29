package aiconfig

import "futrixdata/platform/internal/platform"

// Handler manages AI configuration HTTP endpoints.
type Handler struct {
	store          *Store
	tester         func(AIConfig) TestResult
	onStatusUpdate func(AIConfig, TestResult)
}

// NewHandler creates a new AI config handler.
func NewHandler(store *Store) *Handler {
	return &Handler{
		store: store,
		tester: func(AIConfig) TestResult {
			return TestResult{Connected: false, Error: "tester not configured"}
		},
	}
}

// SetTester sets the connection tester function.
func (h *Handler) SetTester(tester func(AIConfig) TestResult) {
	h.tester = tester
}

func (h *Handler) SetStatusObserver(observer func(AIConfig, TestResult)) {
	h.onStatusUpdate = observer
}

// RegisterRoutes registers all AI config routes.
func (h *Handler) RegisterRoutes(srv *platform.Server) {
	srv.HandleFunc("/api/aiconfigs", h.handleAIConfigs)
	srv.HandleFunc("/api/aiconfigs/", h.handleAIConfigByID)
	srv.HandleFunc("/api/aiconfigs/providers", h.handleProviders)
	srv.HandleFunc("/api/aiconfigs/test", h.handleTestRequest)
}
