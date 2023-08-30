/*
Package rclone contains all the rclone operations
*/
package rclone

import (
	"encoding/json"
	"fmt"
	"time"

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
func Clone(source string, destination string) {
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
		fmt.Println(err)
	}
	out, status := librclone.RPC("sync/sync", string(syncRequestJSON))
	if status != 200 {
		fmt.Printf("Error: Got status : %d and output %q", status, out)
	}
	var sres syncResponse
	err = json.Unmarshal([]byte(out), &sres)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Job Id of Async Job: %d\n", sres.JobID)

	statusReq := statusRequest{JobID: sres.JobID}
	statusRequestJSON, err := json.Marshal(statusReq)
	if err != nil {
		fmt.Println(err)
	}
	var statusResp statusResponse

	statusTries := 0
	for !statusResp.Finished {
		cout, status := librclone.RPC("job/status", string(statusRequestJSON))
		fmt.Println(cout)
		if status == 404 {
			fmt.Println("Job not found!")
			break
		}
		err = json.Unmarshal([]byte(cout), &statusResp)
		if err != nil {
			fmt.Println(err)
			break
		}
		time.Sleep(time.Second)
		statusTries++
		fmt.Printf("Polled status of job %d, %d times\n", statusReq.JobID, statusTries)
	}

	if !statusResp.Success {
		fmt.Println("Job finished but did not have status success.")
		return
	}

	statsReq := statsRequest{Group: "MyTransfer"}

	statsRequestJSON, err := json.Marshal(statsReq)
	if err != nil {
		fmt.Println(err)
	}

	out, _ = librclone.RPC("core/stats", string(statsRequestJSON))
	var stats statsResponse

	err = json.Unmarshal([]byte(out), &stats)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("Transferred %d bytes and %d files\n", stats.Bytes, stats.Transfers)
}
