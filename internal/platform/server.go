package platform

import (
	"net/http"
)

type Server struct {
	addr     string
	mux      *http.ServeMux
	indexDoc []byte
}

func NewServer(addr string) *Server {
	return &Server{addr: addr, mux: http.NewServeMux()}
}

func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	s.mux.HandleFunc(pattern, handler)
}

func (s *Server) SetIndex(index []byte) {
	s.indexDoc = index
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.indexDoc)
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
