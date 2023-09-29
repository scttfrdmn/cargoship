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
	URL          *url.URL
	Token        string
	client       *http.Client
	printCurl    bool
	skipFinalize bool
}

// TravelAgenter is the thing that describes what a travel agent is!
type TravelAgenter interface {
	StatusURL() string
	Update(StatusUpdate) (*StatusUpdateResponse, error)
}

// StatusUpdate is a little structure that gives our TravelAgent more info on
// where we are in the process
type StatusUpdate struct {
	Status                 Status     `json:"status,omitempty"`
	SizeBytes              int64      `json:"size_bytes,omitempty"`
	TransferredBytes       int64      `json:"transferred_bytes,omitempty"`
	Name                   string     `json:"-"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	MetadataCheckSum       string     `json:"metadata_checksum,omitempty"`
	Metadata               string     `json:"metadata,omitempty"`
	SuitcasectlSource      string     `json:"suitcasectl_source,omitempty"`
	SuitcasectlDestination string     `json:"suitcasectl_destination,omitempty"`
}

// Validate the built in TravelAgent meets the TravelAgenter interface
var _ TravelAgenter = TravelAgent{}

// StatusURL is just the web url for viewing this stuff
func (t TravelAgent) StatusURL() string {
	pathPieces := strings.Split(t.URL.Path, "/")
	id := pathPieces[len(pathPieces)-1]
	return fmt.Sprintf("https://%v/suitcase_transfers/%v", t.URL.Host, id)
}

// componentURL is the endpoint for a given component to send to
func (t TravelAgent) componentURL(n string) string {
	return fmt.Sprintf("%v/suitcase_components/%v", t.URL, n)
}

// Update updates the status of an agent
func (t TravelAgent) Update(s StatusUpdate) (*StatusUpdateResponse, error) {
	// Just skip
	if t.skipFinalize && (s.Status == StatusComplete) {
		return &StatusUpdateResponse{
			Messages: []string{"doing a fake complete since we said skip-finalize"},
		}, nil
	}
	var r StatusUpdateResponse
	body, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var req *http.Request
	if s.Name != "" {
		req, err = http.NewRequest("PATCH", t.componentURL(s.Name), bytes.NewReader(body))
	} else {
		req, err = http.NewRequest("PATCH", t.URL.String(), bytes.NewReader(body))
	}
	if err != nil {
		return nil, err
	}

	_, err = t.sendRequest(req, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
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
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return nil, errors.New(strings.Join(errRes.Errors, ", "))
		}
		// If we couldn't parse the error message
		return nil, fmt.Errorf("error, status code: %d", res.StatusCode)
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

	skipFinalize, serr := cmd.Flags().GetBool("travel-agent-skip-finalize")
	if serr != nil {
		return failure(serr)
	}

	return success(func(t *TravelAgent) {
		if endpoint != nil {
			t.URL = endpoint
		}
		if token != "" {
			t.Token = token
		}
		t.skipFinalize = skipFinalize
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

// NewStatusUpdate returns a new status update from an rclone.TransferStatus object
func NewStatusUpdate(r rclone.TransferStatus) *StatusUpdate {
	s := &StatusUpdate{}
	if r.Name != "" {
		s.Name = r.Name
	}
	if r.Stats.Bytes != 0 {
		s.TransferredBytes = r.Stats.Bytes
	}
	if r.Stats.TotalBytes != 0 {
		s.SizeBytes = r.Stats.TotalBytes
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
	Errors []string `json:"errors"`
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
	cmd.PersistentFlags().Bool("travel-agent-skip-finalize", false, "Use this to prevent a 'complete' status from being sent. Useful for debugging")

	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-url")
	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-token")
}
