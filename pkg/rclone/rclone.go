package rclone

import (
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/rclone/rclone/backend/all"
	_ "github.com/rclone/rclone/fs/sync"
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

func Clone(source string, destination string) {
	// We are pushing all the usage to Stdout instead of Stderr. I would
	// like to eventually get this back to stderr, however currently that
	// breaks the shell completion pieces, as all shells expect them on
	// stdout. Hopefully cobra will be able to have multiple outputs at some
	// point
	librclone.Initialize()
	syncRequest := syncRequest{
		SrcFs: source,
		DstFs: destination,
		Group: "MyTransfer",
		Async: true,
	}

	syncRequestJSON, err := json.Marshal(syncRequest)
	if err != nil {
		fmt.Println(err)
	}
	out, status := librclone.RPC("sync/sync", string(syncRequestJSON))
	if status != 200 {
		fmt.Printf("Error: Got status : %d and output %q", status, out)
	}
	var syncResponse syncResponse
	err = json.Unmarshal([]byte(out), &syncResponse)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Job Id of Async Job: %d\n", syncResponse.JobID)

	statusRequest := statusRequest{JobID: syncResponse.JobID}
	statusRequestJSON, err := json.Marshal(statusRequest)
	if err != nil {
		fmt.Println(err)
	}
	var statusResponse statusResponse

	statusTries := 0
	for !statusResponse.Finished {
		out, status := librclone.RPC("job/status", string(statusRequestJSON))
		fmt.Println(out)
		if status == 404 {
			fmt.Println("Job not found!")
			break
		}
		err = json.Unmarshal([]byte(out), &statusResponse)
		if err != nil {
			fmt.Println(err)
			break
		}
		time.Sleep(time.Second)
		statusTries++
		fmt.Printf("Polled status of job %d, %d times\n", statusRequest.JobID, statusTries)
	}

	if !statusResponse.Success {
		fmt.Println("Job finished but did not have status success.")
		return
	}

	statsRequest := statsRequest{Group: "MyTransfer"}

	statsRequestJSON, err := json.Marshal(statsRequest)
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
