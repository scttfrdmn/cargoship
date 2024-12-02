package travelagent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type suitcaseTransferState struct {
	Status Status
	Size   int64
}

// Server holds on the stuff that serves up a travel agent
type Server struct {
	listener        net.Listener
	adminToken      string
	staticTransfers []credentialResponse
	db              *bolt.DB
	dbf             string
}

// WithDBFile sets the database filename for a new server. If no filename is set, we'll use a temp file
func WithDBFile(f string) func(*Server) {
	return func(s *Server) {
		s.dbf = f
	}
}

// WithStaticTransfers sets the static transfers for a given server
func WithStaticTransfers(t []credentialResponse) func(*Server) {
	for _, item := range t {
		if item.Destination == "" {
			fmt.Fprintf(os.Stderr, "IMPORT: %+v\n", item)
			panic("missing destination")
		}
	}
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

	if s.dbf == "" {
		tf, err := os.MkdirTemp("", "travelagent-server")
		panicIfErr(err)
		s.dbf = path.Join(tf, "server.db")
	}

	var dberr error
	s.db, dberr = bolt.Open(s.dbf, 0o600, nil)
	panicIfErr(dberr)
	ierr := s.initDB()
	panicIfErr(ierr)

	if s.adminToken == "" {
		guuid := fmt.Sprint(uuid.New())
		slog.Warn("setting a random admin token", "token", guuid)
		s.adminToken = guuid
	}
	return s
}

func (s *Server) getState(id int, name string) (*suitcaseTransferState, error) {
	var ret suitcaseTransferState

	if err := s.db.View(func(tx *bolt.Tx) error {
		b := s.getBucket(tx).Get([]byte(path.Join(fmt.Sprint(id), name)))
		if string(b) == "" {
			return nil
		}
		if err := json.Unmarshal(b, &ret); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &ret, nil
}

func (s *Server) setState(id int, name string, state suitcaseTransferState) error {
	stateB, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		err := s.getBucket(tx).Put([]byte(path.Join(fmt.Sprint(id), name)), stateB)
		return err
	})
}

func (s Server) getBucket(tx *bolt.Tx) *bolt.Bucket {
	return tx.Bucket([]byte("suitcases"))
}

// initDB initializes the database
func (s *Server) initDB() error {
	// store some data
	bn := []byte("suitcases")
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := s.getBucket(tx)
		if bucket == nil {
			if _, berr := tx.CreateBucket(bn); berr != nil {
				return berr
			}
		}
		return nil
	})
}

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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(200)
	ret, err := json.Marshal(s.staticTransfers[id])
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
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

func getUpdatedFields(old suitcaseTransferState, newStatus StatusUpdate, name string) []string {
	updatedFields := []string{}
	if newStatus.Status != old.Status {
		slog.Info("Updating status", "old", old.Status, "new", newStatus.Status, "item", name)
		updatedFields = append(updatedFields, "status")
	}
	if newStatus.SizeBytes != old.Size {
		slog.Info("Updating size", "old", old.Size, "new", newStatus.SizeBytes, "item", name)
		updatedFields = append(updatedFields, "size")
	}
	return updatedFields
}

func (s *Server) handleStatusUpdate(w http.ResponseWriter, r *http.Request) {
	s.validateToken(w, r)
	id, err := strconv.Atoi(r.PathValue("id"))
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

	/*
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
		}
	*/

	var updatedState StatusUpdate
	if uerr := json.NewDecoder(r.Body).Decode(&updatedState); uerr != nil {
		// if uerr := json.Unmarshal(body, &updatedState); uerr != nil {
		writeErr(w, http.StatusBadRequest, uerr)
		return
	}

	previous, err := s.getState(id, name)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	updatedFields := getUpdatedFields(*previous, updatedState, name)

	if len(updatedFields) > 0 {
		if err := s.setState(id, name, suitcaseTransferState{
			Status: updatedState.Status,
			Size:   updatedState.SizeBytes,
		}); err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
	}

	// currentState.state.status = r.
	if err := writeOK(w, StatusUpdateResponse{
		Messages: []string{
			fmt.Sprintf("updated fields: %v", strings.Join(updatedFields, ", ")),
		},
	}); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
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
	AdminToken string               `json:"admin_token,omitempty" yaml:"admin_token,omitempty"`
	Transfers  []credentialResponse `json:"transfers,omitempty" yaml:"transfers,omitempty"`
}
