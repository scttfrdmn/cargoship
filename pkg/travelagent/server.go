package travelagent

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// staticSuitcaseTransfer is a hard coded transfer for a client
type staticSuitcaseTransfer struct {
	state    map[string]*suitcaseTransferState
	response credentialResponse
}

type suitcaseTransferState struct {
	status Status
	size   int64
}

// Server holds on the stuff that serves up a travel agent
type Server struct {
	listener        net.Listener
	adminToken      string
	staticTransfers []staticSuitcaseTransfer
}

// WithStaticTransfers sets the static transfers for a given server
func WithStaticTransfers(t []staticSuitcaseTransfer) func(*Server) {
	return func(s *Server) {
		s.staticTransfers = t
	}
}

// WithAdminToken sets the admin token for a server
func WithAdminToken(t string) func(*Server) {
	return func(s *Server) {
		s.adminToken = t
	}
}

// WithListener sets the listener on a server object
func WithListener(l net.Listener) func(*Server) {
	return func(s *Server) {
		s.listener = l
	}
}

/*
// WithHTTPServer sets the http server on a Server
func WithHTTPServer(hs *http.Server) func(*Server) {
	return func(s *Server) {
		s.httpServer = hs
	}
}
*/

// NewServer returns a new server with functional options
func NewServer(opts ...func(*Server)) *Server {
	defaultListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	s := &Server{
		listener: defaultListener,
		// httpServer: defaultServer,
	}
	for _, opt := range opts {
		opt(s)
	}

	if s.adminToken == "" {
		guuid := fmt.Sprint(uuid.New())
		slog.Warn("setting a random admin token", "token", guuid)
		s.adminToken = guuid
	}
	return s
}

/*
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	s.validateToken(w, r)
	switch {
	// Credential query
	case strings.HasPrefix(r.URL.Path, "/api/v1/suitcase_transfers") && strings.HasSuffix(r.URL.Path, "/credentials"):
		s.returnCredentials(w, r)
	// Status update
	case strings.HasPrefix(r.URL.Path, "/api/v1/suitcase_transfers"):
		s.processStatusUpdate(w, r)
	default:
		slog.Info("could not find url", "path", r.URL.Path)
		http.NotFound(w, r)
	}
}
*/

func notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(404)
	resB, merr := json.Marshal(ErrorResponse{
		Errors: []string{fmt.Sprintf("route not found: %v", r.URL)},
	})
	panicIfErr(merr)
	_, werr := w.Write(resB)
	panicIfErr(werr)
}

// Run runs a given  router and starts the HTTP server
func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/suitcase_transfers/{id}/credentials", s.handleCredentials)
	mux.HandleFunc("PATCH /api/v1/suitcase_transfers/{id}/suitcase_components/{name}", s.handleStatusUpdate)
	mux.HandleFunc("PATCH /api/v1/suitcase_transfers/{id}", s.handleStatusUpdate)
	mux.HandleFunc("/", notFound)
	hs := &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}
	return hs.Serve(s.listener)
}

// Port returns the port that a server is listening on
func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Addr returns the listening address
func (s Server) Addr() net.Addr {
	return s.listener.Addr()
}

func (s Server) validateToken(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %v", s.adminToken) {
		w.WriteHeader(401)
		resB, merr := json.Marshal(ErrorResponse{
			Errors: []string{"invalid token"},
		})
		panicIfErr(merr)
		_, werr := w.Write(resB)
		panicIfErr(werr)
		return
	}
}

func (s Server) handleCredentials(w http.ResponseWriter, r *http.Request) {
	s.validateToken(w, r)
	dest, err := os.MkdirTemp("", "travelagent-poc")
	panicIfErr(err)
	w.WriteHeader(200)
	ret, err := json.Marshal(credentialResponse{
		AuthType:      map[string]string{},
		Destination:   dest,
		ExpireSeconds: 600,
	})
	panicIfErr(err)
	_, werr := w.Write(ret)
	panicIfErr(werr)
}

func writeIfErr(w http.ResponseWriter, code int, err error) {
	if err != nil {
		writeErr(w, code, err)
	}
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.WriteHeader(code)
	res := ErrorResponse{
		Errors: []string{err.Error()},
	}
	b, err := json.Marshal(res)
	panicIfErr(err)
	_, werr := w.Write(b)
	panicIfErr(werr)
}

func (s *Server) handleStatusUpdate(w http.ResponseWriter, r *http.Request) {
	s.validateToken(w, r)
	idS := r.PathValue("id")
	id, err := strconv.Atoi(idS)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
	}

	name := r.PathValue("name")
	if name == "" {
		name = "TOTAL"
	}

	if len(s.staticTransfers) < id+1 {
		writeErr(w, http.StatusNotFound, fmt.Errorf("invalid transfer id: %v", id))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
	}

	var updatedState StatusUpdate
	// if err := json.NewDecoder(r.Body).Decode(&updatedState); err != nil {
	if err := json.Unmarshal(body, &updatedState); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if s.staticTransfers[id].state == nil {
		s.staticTransfers[id].state = map[string]*suitcaseTransferState{name: {}}
	}

	if _, ok := s.staticTransfers[id].state[name]; !ok {
		s.staticTransfers[id].state[name] = &suitcaseTransferState{}
	}

	updatedFields := []string{}
	if updatedState.Status != s.staticTransfers[id].state[name].status {
		slog.Info("Updating status", "old", s.staticTransfers[id].state[name].status, "new", updatedState.Status)
		s.staticTransfers[id].state[name].status = updatedState.Status
		updatedFields = append(updatedFields, "status")
	}
	if updatedState.SizeBytes != s.staticTransfers[id].state[name].size {
		slog.Info("Updating size", "old", s.staticTransfers[id].state[name].size, "new", updatedState.SizeBytes)
		s.staticTransfers[id].state[name].size = updatedState.SizeBytes
		updatedFields = append(updatedFields, "size")
	}

	// currentState.state.status = r.
	if err := writeOK(w, StatusUpdateResponse{
		Messages: []string{
			fmt.Sprintf("updated fields: %v", strings.Join(updatedFields, ", ")),
		},
	}); err != nil {
		panicIfErr(err)
	}
}

func writeOK(w http.ResponseWriter, v any) error {
	ret, err := json.Marshal(v)
	writeIfErr(w, http.StatusBadRequest, err)
	w.WriteHeader(http.StatusOK)
	_, werr := w.Write(ret)
	return werr
}

// StaticCredentials represents a token and list of credential data.
// Not suitable for production use, just used for testing
type StaticCredentials struct {
	AdminToken string                   `json:"admin_token,omitempty" yaml:"admin_token,omitempty"`
	Transfers  []staticSuitcaseTransfer `json:"transfers,omitempty" yaml:"transfers,omitempty"`
}
