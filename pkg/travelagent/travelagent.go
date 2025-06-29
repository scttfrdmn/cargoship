/*
Package travelagent gets the suitcases to their final destination
*/
package travelagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/sethvargo/go-retry"
	"github.com/spf13/cobra"
	"github.com/scttfrdmn/cargoship-cli/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship-cli/pkg/plugins/transporters/cloud"
	"github.com/scttfrdmn/cargoship-cli/pkg/rclone"
	"moul.io/http2curl"
)

// TravelAgent is the main object that's gonna do all this work
type TravelAgent struct {
	URL          *url.URL
	Token        string
	client       *http.Client
	printCurl    bool
	skipFinalize bool
	// cloudCredentials map[string]string
	// uploadTokenExpiration is a long expiration that should cover the uploading of the largest suitcase
	uploadTokenExpiration time.Duration
	// uploadMetaTokenExpiration is a short expiration used for uploading metadata
	uploadMetaTokenExpiration time.Duration
	uploadRetries             int
	uploadRetryTime           time.Duration
	UniquePrefix              string
	backoff                   retry.Backoff
}

// TravelAgenter is the thing that describes what a travel agent is!
type TravelAgenter interface {
	StatusURL() string
	PostMetaData(string) error
	Update(StatusUpdate) (*StatusUpdateResponse, error)
	Upload(string, chan rclone.TransferStatus) (int64, error)
}

// StatusUpdate is a little structure that gives our TravelAgent more info on
// where we are in the process
type StatusUpdate struct {
	Status                 Status     `json:"status"`
	SizeBytes              int64      `json:"size_bytes,omitempty"`
	Speed                  float64    `json:"speed,omitempty"`
	TransferredBytes       int64      `json:"transferred_bytes,omitempty"`
	PercentDone            int        `json:"percent_done,omitempty"`
	Name                   string     `json:"-"`
	StartedAt              *time.Time `json:"started_at,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	MetadataCheckSum       string     `json:"metadata_checksum,omitempty"`
	Metadata               string     `json:"-"`
	SuitcasectlSource      string     `json:"suitcasectl_source,omitempty"`
	SuitcasectlDestination string     `json:"suitcasectl_destination,omitempty"`
}

// Validate the built in TravelAgent meets the TravelAgenter interface
var _ TravelAgenter = TravelAgent{}

type credentialResponse struct {
	AuthType      map[string]string `json:"auth_type"`
	Destination   string            `json:"destination"`
	ExpireSeconds int               `json:"expire_seconds"`
}

// connectionString returns an rclone style connection string for a given credential
func (c credentialResponse) connectionString() string {
	ctype := "local"
	additionalAuth := map[string]string{}
	for k, v := range c.AuthType {
		if k == "type" {
			ctype = v
		} else {
			additionalAuth[k] = v
		}
	}
	connStr := ":" + ctype
	for k, v := range additionalAuth {
		connStr = fmt.Sprintf("%v,%v='%v'", connStr, k, v)
	}
	return connStr + ":"
}

// StatusURL is just the web url for viewing this stuff
func (t TravelAgent) StatusURL() string {
	pathPieces := strings.Split(t.URL.Path, "/")
	id := pathPieces[len(pathPieces)-1]
	return fmt.Sprintf("https://%v/suitcase_transfers/%v", t.URL.Host, id)
}

func (t TravelAgent) credentialURL() string {
	pathPieces := strings.Split(t.URL.Path, "/")
	id := pathPieces[len(pathPieces)-1]
	if id == "" {
		panic("could not get id")
	}
	return fmt.Sprintf("%v://%v/api/v1/suitcase_transfers/%v/credentials", t.URL.Scheme, t.URL.Host, id)
}

// Upload sends a file off to the cloud, given the file to upload
func (t TravelAgent) Upload(fn string, c chan rclone.TransferStatus) (int64, error) {
	attempt := 0

	if err := retry.Do(context.Background(), t.backoff, func(_ context.Context) error {
		attempt++
		uploadCred, err := t.getCredentials()
		if err != nil {
			return err
		}
		slog.Info("got cloud credentials to upload file",
			"file", path.Base(fn),
			"destination", uploadCred.Destination,
			"expiration", fmt.Sprint(time.Duration(uploadCred.ExpireSeconds)*time.Second),
		)

		trans := cloud.Transporter{
			Config: transporters.Config{
				Destination: uploadCred.connectionString() + uploadCred.Destination,
			},
		}
		if cerr := trans.Check(); cerr != nil {
			return cerr
		}

		if serr := trans.SendWithChannel(fn, t.UniquePrefix, c); serr != nil {
			slog.Warn("upload failed, sleeping then will try again",
				"current-attempt", attempt,
				"max-retries", t.uploadRetries,
				"file", path.Base(fn),
				"error", serr,
			)
			if err := retry.RetryableError(errors.New("could not upload file, will retry")); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	//}
	fstat, err := os.Stat(fn)
	if err != nil {
		return 0, fmt.Errorf("could stat file: %v", fn)
	}
	return fstat.Size(), nil
}

// componentURL is the endpoint for a given component to send to
func (t TravelAgent) componentURL(n string) string {
	return fmt.Sprintf("%v/suitcase_components/%v", t.URL, n)
}

// metadataURL
func (t TravelAgent) metadataURL() string {
	return fmt.Sprintf("%v/metadata_file", t.URL)
}

func (t *TravelAgent) getCredentials() (*credentialResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%v?expiry_seconds=%v", t.credentialURL(), t.uploadTokenExpiration.Seconds()), nil)
	if err != nil {
		return nil, err
	}

	var credentialR credentialResponse
	if _, cerr := t.sendRequest(req, &credentialR); cerr != nil {
		return nil, cerr
	}
	if credentialR.Destination == "" {
		return nil, errors.New("credential response did not specify a destination")
	}

	return &credentialR, nil
}

// PostMetaData posts raw metadata file to travel agent
func (t TravelAgent) PostMetaData(pathName string) error {
	file, oerr := os.Open(path.Clean(pathName))
	if oerr != nil {
		return oerr
	}
	defer dclose(file)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, cerr := writer.CreateFormFile("metadata_file", path.Base(pathName))
	if cerr != nil {
		return cerr
	}

	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", t.metadataURL(), body)
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", writer.FormDataContentType())
	var r StatusUpdateResponse
	if _, err := t.sendRequest(req, &r); err != nil {
		return nil
	}
	return nil
}

// Update updates the status of an agent
func (t TravelAgent) Update(s StatusUpdate) (*StatusUpdateResponse, error) {
	// In case we don't wanna truly finalize...
	if t.skipFinalize && (s.Status == StatusComplete) {
		log.Warn("keeping status as InProgress since we want to skip the finalize", "component", s.Name)
		s.Status = StatusInProgress
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

func (t *TravelAgent) sendRequest(req *http.Request, v interface{}) (*Response, error) { // nolint:unparam
	bearer := "Bearer " + t.Token
	req.Header.Add("Authorization", bearer)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	if t.printCurl {
		command, _ := http2curl.GetCurlCommand(req)
		fmt.Fprintf(os.Stderr, "%v\n", command)
	}

	res, err := t.client.Do(req)
	req.Close = true
	if err != nil {
		return nil, err
	}

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

// WithUniquePrefix adds a unique prefix to the uploaded path
func WithUniquePrefix(s string) Option {
	return success(func(t *TravelAgent) {
		t.UniquePrefix = s
	})
}

// WithURL sets the url from a string
func WithURL(s string) Option {
	s = strings.TrimSuffix(s, "/")
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

// WithCredentialBlob sets the url and password from a base64 encoded json blob
func WithCredentialBlob(s string) Option {
	creds, err := blobToCred(s)
	if err != nil {
		return failure(err)
	}
	cURL, err := url.Parse(creds.URL)
	if err != nil {
		return failure(err)
	}
	return success(func(t *TravelAgent) {
		t.Token = creds.Token
		t.URL = cURL
	})
}

// WithClient sets the http client
func WithClient(c *http.Client) Option {
	return success(func(t *TravelAgent) {
		t.client = c
	})
}

// WithTokenExpiration sets the suitcase token at create
func WithTokenExpiration(d time.Duration) Option {
	return success(func(t *TravelAgent) {
		t.uploadTokenExpiration = d
	})
}

// WithMetaTokenExpiration sets the suitcase token at create
func WithMetaTokenExpiration(d time.Duration) Option {
	return success(func(t *TravelAgent) {
		t.uploadMetaTokenExpiration = d
	})
}

// WithUploadRetries sets the total retries for an upload
func WithUploadRetries(i int) Option {
	return success(func(t *TravelAgent) {
		t.uploadRetries = i
	})
}

// WithUploadRetryTime sets the time between retries
func WithUploadRetryTime(d time.Duration) Option {
	return success(func(t *TravelAgent) {
		t.uploadRetryTime = d
	})
}

// WithCmd binds cobra args on
func WithCmd(cmd *cobra.Command) Option {
	credBlob, err := cmd.Flags().GetString("travel-agent")
	if err != nil {
		return failure(err)
	}
	credExpire, err := cmd.Flags().GetDuration("travel-agent-credential-expiration")
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
		t.uploadTokenExpiration = credExpire
	})
}

// New returns a new TravelAgent using functional options
func New(options ...Option) (*TravelAgent, error) {
	ta := &TravelAgent{
		client:                    http.DefaultClient,
		uploadTokenExpiration:     24 * time.Hour,
		uploadMetaTokenExpiration: 1 * time.Hour,
		uploadRetries:             5,
		uploadRetryTime:           time.Minute * 2,
	}
	for _, option := range options {
		opt, err := option()
		if err != nil {
			return nil, err
		}
		opt(ta)
	}

	// Assemble the backoff based on retries
	ta.backoff = retry.NewFibonacci(ta.uploadRetryTime)
	ta.backoff = retry.WithMaxRetries(intToUint64(ta.uploadRetries), ta.backoff)
	// ta.backoff = retry.WithMaxDuration(5*time.Minute, ta.backoff)

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
	s := &StatusUpdate{
		StartedAt: r.Status.StartTime,
	}
	if r.Name != "" {
		s.Name = r.Name
	}
	switch {
	case len(r.Stats.Transferring) == 1:

		// Dividing percentage and Transferred by 2x. The raw stats
		// appear to be roughly double the actual values. Unsure if
		// this is a bug or if the read from the disk is also being
		// counted here
		s.PercentDone = r.Stats.Transferring[0].Percentage / 2
		if r.Stats.Transferring[0].Bytes > 0 {
			s.TransferredBytes = r.Stats.Transferring[0].Bytes / 2
		}
		if r.Stats.Transferring[0].Size > 0 {
			s.SizeBytes = r.Stats.Transferring[0].Size
		}
		if r.Stats.Transferring[0].SpeedAvg > 0 {
			s.Speed = r.Stats.Transferring[0].SpeedAvg
		}

	case len(r.Stats.Transferring) > 1:
		log.Warn("hmmm...found multiple files uploading...we aren't handling this correctly", "trans", r.Stats.Transferring)
	}

	switch {
	case !r.Status.Finished:
		s.Status = StatusInProgress
	case r.Status.Finished && r.Status.Success:
		s.CompletedAt = nowPtr()
		s.Status = StatusComplete
		s.SizeBytes = r.Stats.TotalBytes / 2
		s.TransferredBytes = r.Stats.TotalBytes / 2
	case r.Status.Finished && !r.Status.Success:
		s.Status = StatusFailed
	default:
		panic("how did we get here??")
	}

	if (r.Status.EndTime != nil) && !r.Status.EndTime.IsZero() {
		s.CompletedAt = r.Status.EndTime
	}
	return s
}

func nowPtr() *time.Time {
	n := time.Now()
	return &n
}

// Status describes specific statuses for the updates
type Status int

// MarshalJSON handles converting the int to a string
func (s Status) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%v\"", s.String())), nil
}

// UnmarshalJSON does the correct conversion for unmarshalling a status
func (s *Status) UnmarshalJSON(data []byte) error {
	var err error
	*s, err = StatusString(strings.TrimPrefix(strings.TrimSuffix(string(data), "\""), "\""))
	return err
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

// StatusString returns a Status using a string
func StatusString(s string) (Status, error) {
	switch s {
	case "pending":
		return StatusPending, nil
	case "in_progress":
		return StatusInProgress, nil
	case "complete":
		return StatusComplete, nil
	case "failed":
		return StatusFailed, nil
	default:
		return StatusPending, fmt.Errorf("unknown status string: %v", s)
	}
}

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
	cmd.PersistentFlags().Duration("travel-agent-credential-expiration", 24*time.Hour, "Expiration time for the token generated by your TravelAgent to upload to the cloud")
	cmd.PersistentFlags().Bool("travel-agent-skip-finalize", false, "Use this to prevent a 'complete' status from being sent. Useful for debugging")

	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-url")
	cmd.MarkFlagsMutuallyExclusive("travel-agent", "travel-agent-token")
}

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func intToUint64(i int) uint64 {
	if i < 0 {
		panic("value is negative and cannot be converted to uint64")
	}
	return uint64(i)
}
