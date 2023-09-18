/*
Package travelagent gets the suitcases to their final destination
*/
package travelagent

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
	"moul.io/http2curl"
)

// TravelAgent is the main object that's gonna do all this work
type TravelAgent struct {
	URL       *url.URL
	Token     string
	client    *http.Client
	printCurl bool
}

// StatusURL is just the web url for viewing this stuff
func (t TravelAgent) StatusURL() string {
	pathPieces := strings.Split(t.URL.Path, "/")
	id := pathPieces[len(pathPieces)-1]
	return fmt.Sprintf("https://%v/suitcase_transfers/%v", t.URL.Host, id)
}

type credential struct {
	URL   string `json:"url"`
	Token string `json:"password"`
}

func blobToCred(b string) (*credential, error) {
	text, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return nil, err
	}
	var c credential
	err = json.Unmarshal(text, &c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Update updates the status of an agent
func (t TravelAgent) Update(s StatusUpdate) (*StatusUpdateResponse, error) {
	var r StatusUpdateResponse
	body, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("PATCH", t.URL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	_, err = t.sendRequest(req, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (t *TravelAgent) sendRequest(req *http.Request, v interface{}) (*Response, error) {
	bearer := "Bearer " + t.Token
	req.Header.Add("Authorization", bearer)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	if t.printCurl {
		command, _ := http2curl.GetCurlCommand(req)
		fmt.Fprintf(os.Stderr, "%v\n", command)
	}

	res, err := t.client.Do(req)
	req.Close = true
	if err != nil {
		return nil, err
	}
	// b, _ := ioutil.ReadAll(res.Body)

	defer dclose(res.Body)

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		var errRes ErrorResponse
		// b, _ := io.ReadAll(res.Body)
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return nil, errors.New(errRes.Message)
		}

		switch res.StatusCode {
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("too many requests.  Check rate limit and make sure the userAgent is set right")
		case http.StatusNotFound:
			return nil, fmt.Errorf("that entry was not found, are you sure it exists?")
		default:
			return nil, fmt.Errorf("error, status code: %d", res.StatusCode)
		}
	}

	b, _ := io.ReadAll(res.Body)
	if string(b) != "" {
		if err = json.NewDecoder(bytes.NewReader(b)).Decode(&v); err != nil {
			return nil, err
		}
	}
	r := &Response{Response: res}

	return r, nil
}

// Option is just an option for TravelAgent
type Option func() (func(*TravelAgent), error)

func success(opt func(*TravelAgent)) Option {
	return func() (func(*TravelAgent), error) {
		return opt, nil
	}
}

func failure(err error) Option {
	return func() (func(*TravelAgent), error) {
		return nil, err
	}
}

// WithURL sets the url from a string
func WithURL(s string) Option {
	u, err := url.Parse(s)
	if err != nil {
		return failure(err)
	}
	return success(func(t *TravelAgent) {
		t.URL = u
	})
}

// WithPrintCurl prints out the curl command for each request
func WithPrintCurl() Option {
	return success(func(t *TravelAgent) {
		t.printCurl = true
	})
}

// WithToken sets the token
func WithToken(s string) Option {
	return success(func(t *TravelAgent) {
		t.Token = s
	})
}

// WithClient sets the http client
func WithClient(c *http.Client) Option {
	return success(func(t *TravelAgent) {
		t.client = c
	})
}

// WithCmd binds cobra args on
func WithCmd(cmd *cobra.Command) Option {
	credBlob, err := cmd.Flags().GetString("travel-agent")
	if err != nil {
		return failure(err)
	}
	var endpoint *url.URL
	var token string
	if credBlob != "" {
		cred, err := blobToCred(credBlob)
		if err != nil {
			return failure(err)
		}
		token = cred.Token
		endpoint, err = url.Parse(cred.URL)
		if err != nil {
			return failure(err)
		}
	} else {
		urlS, err := cmd.Flags().GetString("travel-agent-url")
		if err != nil {
			return failure(err)
		}
		endpoint, err = url.Parse(urlS)
		if err != nil {
			return failure(err)
		}
		token, err = cmd.Flags().GetString("travel-agent-token")
		if err != nil {
			return failure(err)
		}
	}

	return success(func(t *TravelAgent) {
		if endpoint != nil {
			t.URL = endpoint
		}
		if token != "" {
			t.Token = token
		}
	})
}

// New returns a new TravelAgent using functional options
func New(options ...Option) (*TravelAgent, error) {
	ta := &TravelAgent{
		client: http.DefaultClient,
	}
	for _, option := range options {
		opt, err := option()
		if err != nil {
			return nil, err
		}
		opt(ta)
	}

	if os.Getenv("DEBUG_CURL") != "" {
		ta.printCurl = true
	}

	if ta.URL == nil {
		return nil, errors.New("must set a URL")
	}
	if ta.Token == "" {
		return nil, errors.New("must set a token")
	}

	return ta, nil
}

// StatusUpdate is a little structure that gives our TravelAgent more info on
// where we are in the process
type StatusUpdate struct {
	Status                    Status     `json:"status,omitempty"`
	SizeBytes                 int64      `json:"size_bytes,omitempty"`
	TransferredBytes          int64      `json:"transferred_bytes,omitempty"`
	ComponentName             string     `json:"component_name,omitempty"`
	ComponentSizeBytes        int64      `json:"component_size_bytes,omitempty"`
	ComponentTransferredBytes int64      `json:"component_transferred_bytes,omitempty"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	CompletedAt               *time.Time `json:"completed_at,omitempty"`
	MetadataCheckSum          string     `json:"metadata_checksum,omitempty"`
	Metadata                  string     `json:"metadata,omitempty"`
	SuitcasectlSource         string     `json:"suitcasectl_source,omitempty"`
	SuitcasectlDestination    string     `json:"suitcasectl_destination,omitempty"`
}

// NewStatusUpdate returns a new status update from an rclone.TransferStatus object
func NewStatusUpdate(r rclone.TransferStatus) *StatusUpdate {
	s := &StatusUpdate{}
	if r.Name != "" {
		s.ComponentName = r.Name
	}
	if r.Stats.Bytes != 0 {
		s.ComponentTransferredBytes = r.Stats.Bytes
	}
	if r.Stats.TotalBytes != 0 {
		s.ComponentSizeBytes = r.Stats.TotalBytes
	}
	if !r.Status.Finished {
		s.Status = StatusInProgress
	}
	return s
}

// Status describes specific statuses for the updates
type Status int

// MarshalJSON handles converting the int to a string
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%v\"", s.String())), nil
}

const (
	// StatusPending has not yet started
	StatusPending = iota
	// StatusInProgress is currently running
	StatusInProgress
	// StatusComplete is completed successfully
	StatusComplete
	// StatusFailed is a falure
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusInProgress:
		return "in_progress"
	case StatusComplete:
		return "complete"
	case StatusFailed:
		return "failed"
	default:
		panic("unknown status")
	}
}

// Response is a good http response
type Response struct {
	Response *http.Response
}

// StatusUpdateResponse is the actual text from a response
type StatusUpdateResponse struct {
	Messages []string `json:"messages,omitempty"`
}

// ErrorResponse represents an error from the api
type ErrorResponse struct {
	Message string `json:"errors"`
}

func dclose(c io.Closer) {
	err := c.Close()
	if err != nil {
		fmt.Fprint(os.Stderr, "error closing item")
	}
}

// BindCobra adds the appropriate flags to use the travel agent
func BindCobra(cmd *cobra.Command) {
	cmd.PersistentFlags().String("travel-agent", "", "Base64 Encoded token and url for the travel agent, in json (Copy paste this from the travel agent website)")
	cmd.PersistentFlags().String("travel-agent-url", "", "URL to use for travel agent operations")
	cmd.PersistentFlags().String("travel-agent-token", "", "Token to use for travel agent operations")

	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-url")
	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-token")
}
