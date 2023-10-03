/*
Package rclone contains all the rclone operations
*/
package rclone

import (
	"encoding/json"
	"errors"
	"fmt"
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
	Filter    string `json:"_filter"`
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
	JobID int64  `json:"jobid,omitempty"`
	Group string `json:"group,omitempty"`
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

type jobStatus struct {
	Duration  float64    `json:"duration,omitempty"`
	StartTime *time.Time `json:"startTime,omitempty"`
	EndTime   *time.Time `json:"endTime,omitempty"`
	Error     string     `json:"error,omitempty"`
	Finished  bool       `json:"finished,omitempty"`
	Group     string     `json:"group,omitempty"`
	ID        float64    `json:"id,omitempty"`
	Output    any        `json:"output,omitempty"` // Not sure what this actually is for...
	Success   bool       `json:"success,omitempty"`
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

type jobStats struct {
	Bytes        int64          `json:"bytes"`
	TotalBytes   int64          `json:"totalBytes"`
	Speed        float64        `json:"speed"`
	Transfers    int64          `json:"transfers,omitempty"`
	ElapsedTime  float64        `json:"elapsedTime,omitempty"`
	Errors       int64          `json:"errors,omitempty"`
	ETA          float64        `json:"eta,omitempty"`
	Transferring []jobFileStats `json:"transferring,omitempty"`
}

type jobFileStats struct {
	Bytes      int64   `json:"bytes"`
	ETA        float64 `json:"eta,omitempty"`
	Name       string  `json:"name"`
	Percentage int     `json:"percentage"`
	Speed      float64 `json:"speed"`
	SpeedAvg   float64 `json:"speedAvg"`
	Size       int64   `json:"size"`
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
	var remote string
	switch {
	case len(pieces) == 1:
		remote = pieces[0]
	case len(pieces) == 2:
		remote = pieces[1]
	default:
		panic("unknown type of destination")
	}
	ar := aboutRequest{
		Fs:     pieces[0] + ":",
		Remote: strings.TrimPrefix(remote, "/"),
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

// TransferStatus is the status of a transfer, used for reporting back to our
// travel agent or such
type TransferStatus struct {
	Name   string    `json:"name,omitempty"`
	Stats  jobStats  `json:"stats,omitempty"`
	Status jobStatus `json:"status,omitempty"`
}

func marshalParams(p rc.Params) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func mustMarshalParams(p rc.Params) string {
	s, err := marshalParams(p)
	if err != nil {
		panic(err)
	}
	return s
}

func copyParamsWithSrcDest(source, destination string) rc.Params {
	sourceB := filepath.Base(source)
	params := rc.Params{
		"srcFs": filepath.Dir(source),
		// "srcFs": source,
		// "srcRemote": sourceB,
		// "dstFs": path.Join(destination, sourceB),
		"dstFs": destination,
		// "dstRemote": sourceB,
		"_async":  true,
		"_group":  sourceB,
		"_filter": fmt.Sprintf(`{"IncludeRule":["%v"]}`, sourceB),
	}
	return params
}

func syncResWithOut(o string) syncResponse {
	var r syncResponse
	err := json.Unmarshal([]byte(o), &r)
	panicIfError(err)
	return r
}

// Copy copies a single file to the destination
func Copy(source, destination string, c chan TransferStatus) error {
	log := log.With().Str("source", source).Str("destination", destination).Logger()
	librclone.Initialize()

	// params := copyParamsWithSrcDest(source, destination)
	params := copyParamsWithSrcDest(source, destination)
	log.Debug().Interface("params", params).Send()
	// out, status := librclone.RPC("operations/copyfile", mustMarshalParams(params))
	out, status := librclone.RPC("sync/copy", mustMarshalParams(params))
	if status != 200 {
		return errWithRPCOut(out)
	}
	syncR := syncResWithOut(out)
	log.Debug().Int64("id", syncR.JobID).Msg("job id of async job")
	statusResp, err := waitForFinished(
		statusRequest{
			JobID: syncR.JobID,
			Group: filepath.Base(source),
		}, c,
	)
	if err != nil {
		return err
	}
	if !statusResp.Success {
		return errors.New("job finished but did not have status success")
	}

	return nil
}

func getStats(id string) (*jobStats, error) {
	// jobID := fmt.Sprintf("job/%v", id)
	out, status := librclone.RPC("core/stats", statsRequest{Group: id}.JSONString())
	if status != 200 {
		return nil, errors.New("error getting stats")
	}
	var stats jobStats
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
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

	statusResp, err := waitForFinished(statusRequest{
		JobID: sres.JobID,
		Group: filepath.Base(source),
	}, nil) // nolint // DS - I dunno why this is triggering S1016...
	if err != nil {
		return err
	}
	if !statusResp.Success {
		return errors.New("job finished but did not have status success")
	}

	out, _ = librclone.RPC("core/stats", statsRequest{Group: "MyTransfer"}.JSONString())

	var stats jobStats
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		return err
	}

	log.Info().Int64("bytes", stats.Bytes).Int64("files", stats.Transfers).Msg("transfer complete")

	return nil
}

func waitForFinished(statusReq statusRequest, c chan TransferStatus) (*jobStatus, error) {
	var statusResp *jobStatus
	var statusTries int
	var stats *jobStats
	for (statusResp == nil) || !statusResp.Finished {
		var err error
		// stats, err := getStats(fmt.Sprintf("job/%v", statusReq.JobID))
		if statusReq.Group == "" {
			return nil, errors.New("missing group")
		}
		if stats, err = getStats(statusReq.Group); err != nil {
			return nil, err
		}
		statusResp, err = getJobStatus(statusReq)
		if err != nil {
			return nil, err
		}
		if c != nil {
			c <- TransferStatus{
				Name:   statusReq.Group,
				Stats:  *stats,
				Status: *statusResp,
			}
		}
		time.Sleep(time.Second)
		statusTries++
	}
	// Send one last entry in
	if c != nil {
		c <- TransferStatus{
			Name:   statusReq.Group,
			Stats:  *stats,
			Status: *statusResp,
		}
	}
	return statusResp, nil
}

func getJobStatus(statusReq statusRequest) (*jobStatus, error) {
	statusS, statusCode := librclone.RPC("job/status", statusReq.JSONString())
	if statusCode == 404 {
		return nil, errors.New("job not found")
	}
	var status jobStatus
	err := json.Unmarshal([]byte(statusS), &status)
	if err != nil {
		return nil, err
	}
	if status.Error != "" {
		return nil, errors.New(status.Error)
	}
	return &status, nil
}

type errorOut struct {
	Error string `json:"error,omitempty"`
}

func errWithRPCOut(o string) error {
	var ret errorOut
	if err := json.Unmarshal([]byte(o), &ret); err != nil {
		return nil
	}
	return errors.New(ret.Error)
}

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}
