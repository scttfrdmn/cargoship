/*
Package rclone contains all the rclone operations
*/
package rclone

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	_ "github.com/rclone/rclone/backend/all" // import all backend
	_ "github.com/rclone/rclone/fs/sync"     // import the file sync bits
	"github.com/rclone/rclone/librclone/librclone"
)

type syncRequest struct {
	SrcFs string `json:"srcFs"`
	DstFs string `json:"dstFs"`
	Group string `json:"_group"`
	Async bool   `json:"_async"`
}

type syncResponse struct {
	JobID int64 `json:"jobid"`
}

type statusRequest struct {
	JobID int64 `json:"jobid"`
}

type statusResponse struct {
	Finished bool `json:"finished"`
	Success  bool `json:"success"`
}

type statsRequest struct {
	Group string `json:"group"`
}

type statsResponse struct {
	Bytes       int64   `json:"bytes"`
	Speed       float64 `json:"speed"`
	Transfers   int64   `json:"transfers"`
	ElapsedTime float64 `json:"elapsedTime"`
	Errors      int64   `json:"errors"`
}

// Clone mimics rclonse 'clone' option, given a source and destination
func Clone(source string, destination string) error {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	librclone.Initialize()
	sreq := syncRequest{
		SrcFs: source,
		DstFs: destination,
		Group: "MyTransfer",
		Async: true,
	}

	syncRequestJSON, err := json.Marshal(sreq)
	if err != nil {
		log.Warn().Err(err).Msg("error marshaling syncRequest")
	}
	out, status := librclone.RPC("sync/sync", string(syncRequestJSON))
	if status != 200 {
		log.Info().Interface("out", out).Msg("clone request status succeeded")
	}
	var sres syncResponse
	if jerr := json.Unmarshal([]byte(out), &sres); jerr != nil {
		return errors.New("error unmarshalling syncResponse")
	}

	log.Info().Int64("id", sres.JobID).Msg("job id of async job")

	statusReq := statusRequest{JobID: sres.JobID} // nolint // DS - I dunno why this is triggering...
	statusRequestJSON, err := json.Marshal(statusReq)
	if err != nil {
		log.Warn().Err(err).Msg("issue unmarshalling status request")
	}
	var statusResp statusResponse

	statusTries := 0
	for !statusResp.Finished {
		cout, status := librclone.RPC("job/status", string(statusRequestJSON))
		fmt.Println(cout)
		if status == 404 {
			log.Warn().Msg("job not found!")
			break
		}
		err = json.Unmarshal([]byte(cout), &statusResp)
		if err != nil {
			log.Warn().Err(err).Msg("issue unmarshalling status response")
			break
		}
		time.Sleep(time.Second)
		statusTries++
		log.Info().Int64("job", statusReq.JobID).Int("tries", statusTries).Msg("polling status")
	}

	if !statusResp.Success {
		return errors.New("job finished but did not have status success")
	}

	statsReq := statsRequest{Group: "MyTransfer"}

	statsRequestJSON, err := json.Marshal(statsReq)
	if err != nil {
		return err
	}

	out, _ = librclone.RPC("core/stats", string(statsRequestJSON))
	var stats statsResponse

	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		return err
	}
	log.Info().Int64("bytes", stats.Bytes).Int64("files", stats.Transfers).Msg("transfer complete")

	return nil
}
