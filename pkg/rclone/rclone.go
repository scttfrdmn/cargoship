/*
Package rclone contains all the rclone operations
*/
package rclone

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"

	_ "github.com/rclone/rclone/backend/all" // import all backend
	"github.com/rclone/rclone/fs/rc"
	_ "github.com/rclone/rclone/fs/sync" // import the file sync bits
	"github.com/rclone/rclone/librclone/librclone"
)

// cloneRequest can be either a sync
type cloneRequest struct {
	SrcFs     string `json:"srcFs,omitempty"`
	DstFs     string `json:"dstFs,omitempty"`
	SrcRemote string `json:"srcRemote,omitempty"`
	DstRemote string `json:"dstRemote,omitempty"`
	Group     string `json:"_group"`
	Async     bool   `json:"_async"`
}

type rpcOutput struct {
	Duration  float64    `json:"duration,omitempty"`
	EndTime   *time.Time `json:"endTime,omitempty"`
	Error     string     `json:"error,omitempty"`
	Finished  bool       `json:"finished,omitempty"`
	Group     string     `json:"group,omitempty"`
	ID        float64    `json:"id,omitempty"`
	Output    any        `json:"output,omitempty"` // Not sure what this actually is for...
	StartTime *time.Time `json:"startTime,omitempty"`
	Success   bool       `json:"success,omitempty"`
}

func newRPCOutput(s string) rpcOutput {
	var r rpcOutput
	err := json.Unmarshal([]byte(s), &r)
	if err != nil {
		panic(err)
	}
	return r
}

// JSON returns the json representation in string format. Panic on error. I
// don't _think_ there's a way this actually errors, so feels safe
func (s cloneRequest) JSONString() string {
	js, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(js)
}

type syncResponse struct {
	JobID int64 `json:"jobid"`
}

type statResponse struct {
	Item *statResponseItem `json:"item,omitempty"`
}

type statResponseItem struct {
	IsDir    bool
	MimeType string
	ModTime  string
	Name     string
	Path     string
	Size     int64
}

type statusRequest struct {
	JobID int64 `json:"jobid"`
}

// JSON returns the json representation in string format. Panic on error. I
// don't _think_ there's a way this actually errors, so feels safe
func (s statusRequest) JSONString() string {
	js, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(js)
}

type statusResponse struct {
	Finished bool `json:"finished"`
	Success  bool `json:"success"`
}

type statsRequest struct {
	Group string `json:"group"`
}

// JSON returns the json representation in string format. Panic on error. I
// don't _think_ there's a way this actually errors, so feels safe
func (s statsRequest) JSONString() string {
	js, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(js)
}

type statsResponse struct {
	Bytes       int64   `json:"bytes"`
	Speed       float64 `json:"speed"`
	Transfers   int64   `json:"transfers"`
	ElapsedTime float64 `json:"elapsedTime"`
	Errors      int64   `json:"errors"`
}

func mustNewCloneRequest(options ...func(*cloneRequest)) *cloneRequest {
	got, err := newCloneRequest(options...)
	if err != nil {
		panic(err)
	}
	return got
}

func withSrcFs(s string) func(*cloneRequest) {
	return func(r *cloneRequest) {
		r.SrcFs = s
	}
}

func withDstFs(s string) func(*cloneRequest) {
	return func(r *cloneRequest) {
		r.DstFs = s
	}
}

func withSrcRemote(s string) func(*cloneRequest) {
	return func(r *cloneRequest) {
		r.SrcRemote = s
	}
}

func withDstRemote(s string) func(*cloneRequest) {
	return func(r *cloneRequest) {
		r.DstRemote = s
	}
}

func withGroup(s string) func(*cloneRequest) {
	return func(r *cloneRequest) {
		r.Group = s
	}
}

func newCloneRequest(options ...func(*cloneRequest)) (*cloneRequest, error) {
	r := &cloneRequest{
		Group: "SuitcaseCTLTransfer",
		Async: true,
	}
	for _, opt := range options {
		opt(r)
	}

	if r.SrcFs == "" && r.SrcRemote == "" {
		return nil, errors.New("must set at least SrcFs or SrcRemote")
	}
	if r.DstFs == "" && r.DstRemote == "" {
		return nil, errors.New("must set at least DstFs or DstRemote")
	}
	return r, nil
}

// newCloneRequestWithSrcDest returns the RPC operation string, appropriate json request, and an error
func newCloneRequestWithSrcDst(source, destination string) (string, *cloneRequest, error) {
	sourceStat, err := os.Stat(source)
	if err != nil {
		return "", nil, err
	}
	var cloneAction string
	var sreq *cloneRequest
	if sourceStat.IsDir() {
		log.Debug().Msg("Using sync cloud method")
		cloneAction = "sync/copy"
		sreq = mustNewCloneRequest(
			withSrcFs(source),
			withDstFs(destination),
			withGroup(slug.Make(source)),
		)
	} else {
		log.Info().Msg("Using copyfile cloud method")
		cloneAction = "operations/copyfile"
		sourceB := filepath.Base(source)
		sreq = mustNewCloneRequest(
			withSrcFs(filepath.Dir(source)),
			withSrcRemote(sourceB),
			withDstFs(destination),
			withDstRemote(sourceB),
			withGroup(slug.Make(source)),
		)
	}
	return cloneAction, sreq, nil
}

type aboutRequest struct {
	Fs     string `json:"fs"`
	Remote string `json:"remote"`
}

// JSONString returns the json representation in string format. Panic on error. I
// don't _think_ there's a way this actually errors, so feels safe
func (a aboutRequest) JSONString() string {
	js, err := json.Marshal(a)
	if err != nil {
		panic(err)
	}
	return string(js)
}

// Exists checks to see if a destination exists. This is useful as a pre-flight check
func Exists(d string) bool {
	librclone.Initialize()
	pieces := strings.Split(d, ":")
	if len(pieces) != 2 {
		panic("Unknown type of destination")
	}

	ar := aboutRequest{
		Fs:     pieces[0] + ":",
		Remote: strings.TrimPrefix(pieces[1], "/"),
	}

	out, status := librclone.RPC("operations/stat", ar.JSONString())
	_ = status
	var sr statResponse
	err := json.Unmarshal([]byte(out), &sr)
	if err != nil {
		panic(err)
	}
	return sr.Item != nil
}

/*
type commandReq struct {
	Command string    `json:"command"`
	Args    []string  `json:"args"`
	Opts    rc.Params `json:"opts"`
}

// JSON returns the json representation in string format. Panic on error. I
// don't _think_ there's a way this actually errors, so feels safe
func (c commandReq) JSONString() string {
	js, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(js)
}
*/

// APIOneShot is a generic way to call the API. Good for single commands to
// send through that don't have to wait and such
func APIOneShot(command string, params rc.Params) error {
	librclone.Initialize()

	paramsB, err := json.Marshal(params)
	if err != nil {
		return err
	}
	out, status := librclone.RPC(command, string(paramsB))
	if status != 200 {
		log.Info().Interface("out", out).Msg("command request status failed")
	}
	log.Info().Msg("🎊 All set! 🎊")
	return nil
}

// Clone mimics rclonse 'clone' option, given a source and destination
func Clone(source, destination string) error {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	log := log.With().Str("source", source).Str("destination", destination).Logger()
	if !Exists(destination) {
		log.Debug().Msg("destination does not exist, it may be created during the run")
	}

	librclone.Initialize()

	cloneAction, sreq, err := newCloneRequestWithSrcDst(source, destination)
	if err != nil {
		return err
	}
	log.Debug().Interface("request", sreq).Send()

	out, status := librclone.RPC(cloneAction, sreq.JSONString())
	if status != 200 {
		log.Info().Interface("out", out).Msg("clone request status failed")
	}

	var sres syncResponse
	if jerr := json.Unmarshal([]byte(out), &sres); jerr != nil {
		return errors.New("error unmarshalling syncResponse")
	}

	log.Info().Int64("id", sres.JobID).Msg("job id of async job")

	statusResp, err := waitForFinished(statusRequest{JobID: sres.JobID}) // nolint // DS - I dunno why this is triggering S1016...
	if err != nil {
		return err
	}
	if !statusResp.Success {
		return errors.New("job finished but did not have status success")
	}

	out, _ = librclone.RPC("core/stats", statsRequest{Group: "MyTransfer"}.JSONString())

	var stats statsResponse
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		return err
	}

	log.Info().Int64("bytes", stats.Bytes).Int64("files", stats.Transfers).Msg("transfer complete")

	return nil
}

func waitForFinished(statusReq statusRequest) (*statusResponse, error) {
	var statusResp statusResponse
	var statusTries int
	for !statusResp.Finished {
		cout, status := librclone.RPC("job/status", statusReq.JSONString())
		coutO := newRPCOutput(cout)
		if coutO.Error != "" {
			log.Warn().Err(errors.New(coutO.Error)).Send()
		}
		if status == 404 {
			return nil, errors.New("job not found")
		}

		err := json.Unmarshal([]byte(cout), &statusResp)
		if err != nil {
			return nil, errors.New("issue unmarshalling status response")
		}
		log.Debug().Int64("job", statusReq.JobID).Int("tries", statusTries).Msg("checking status")
		time.Sleep(time.Second)
		statusTries++
	}
	return &statusResp, nil
}
