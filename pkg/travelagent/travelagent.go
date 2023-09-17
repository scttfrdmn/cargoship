/*
Package travelagent gets the suitcases to their final destination
*/
package travelagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// TravelAgent is the main object that's gonna do all this work
type TravelAgent struct {
	URL       *url.URL
	Token     string
	client    *http.Client
	printCurl bool
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
	fmt.Fprintf(os.Stderr, "REEEE: %+v\n", r)
	return &r, nil
}

func (t *TravelAgent) sendRequest(req *http.Request, v interface{}) (*Response, error) {
	bearer := "Bearer " + t.Token
	req.Header.Add("Authorization", bearer)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

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

// Status describes specific statuses for the updates
type Status int

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
