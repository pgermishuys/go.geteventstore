// Copyright 2016 Jet Basrawi. All rights reserved.
//
// Use of this source code is governed by a permissive BSD 3 Clause License
// that can be found in the license file.

package goes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	. "gopkg.in/check.v1"
)

var _ = Suite(&ClientSuite{})

type ClientSuite struct{}

func (s *ClientSuite) SetUpTest(c *C) {
	setup()
}
func (s *ClientSuite) TearDownTest(c *C) {
	teardown()
}

func newTestClient() *Client {
	baseURL, _ := url.Parse(server.URL)
	return &Client{
		client:  http.DefaultClient,
		baseURL: baseURL,
		headers: make(map[string]string),
	}
}

func (s *ClientSuite) TestReadStream(c *C) {
	stream := "some-stream"
	path := fmt.Sprintf("/streams/%s/head/backward/20", stream)
	url := fmt.Sprintf("%s%s", server.URL, path)

	es := CreateTestEvents(2, stream, server.URL, "EventTypeX")
	f, _ := CreateTestFeed(es, url)

	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, "GET")
		fmt.Fprint(w, f.PrettyPrint())
		c.Assert(r.Header.Get("Accept"), Equals, "application/atom+xml")
	})

	feed, resp, _ := client.ReadFeed(url)
	c.Assert(feed.PrettyPrint(), DeepEquals, f.PrettyPrint())
	c.Assert(resp.StatusCode, DeepEquals, http.StatusOK)
}

func (s *ClientSuite) TestUnmarshalFeed(c *C) {
	stream := "unmarshal-feed"
	count := 2

	es := CreateTestEvents(count, stream, server.URL, "EventTypeX")
	url := fmt.Sprintf("%s/streams/%s/head/backward/%d", server.URL, stream, count)

	wf, _ := CreateTestFeed(es, url)
	want := wf.PrettyPrint()

	gf, err := unmarshalFeed(strings.NewReader(want))
	c.Assert(err, IsNil)
	got := gf.PrettyPrint()

	c.Assert(got, DeepEquals, want)
}

func (s *ClientSuite) TestConstructNewClient(c *C) {
	ct, err := NewClient(nil, server.URL)
	got, want := ct.baseURL.String(), server.URL
	c.Assert(err, IsNil)
	c.Assert(got, Equals, want)
}

func (s *ClientSuite) TestConstructNewClientInvalidURL(c *C) {
	invalidURL := ":"
	_, err := NewClient(nil, invalidURL)
	c.Assert(err, ErrorMatches, "parse :: missing protocol scheme")
}

func (s *ClientSuite) TestNewRequest(c *C) {
	reqURL, outURL := "/foo", server.URL+"/foo"
	reqBody := &Event{EventID: "some-uuid", EventType: "SomeEventType", Data: "some-string"}
	eventStructJSON := `{"eventType":"SomeEventType","eventId":"some-uuid","data":"some-string"}`
	outBody := eventStructJSON + "\n"
	req, _ := client.newRequest("GET", reqURL, reqBody)

	// test that the relative url was concatenated
	c.Assert(req.URL.String(), Equals, outURL)

	// test that body was JSON encoded
	body, _ := ioutil.ReadAll(req.Body)
	c.Assert(string(body), Equals, outBody)
}

func (s *ClientSuite) TestRequestsAreSentWithBasicAuthIfSet(c *C) {
	username := "user"
	password := "pass"
	headerStr := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))

	var authFound bool
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		authFound = header == headerStr
		fmt.Fprintf(w, "")
	})

	client.SetBasicAuth("user", "pass")
	streamReader := client.NewStreamReader("something")
	_ = streamReader.Next()
	c.Assert(authFound, Equals, true)
}

func (s *ClientSuite) TestNewRequestWithInvalidJSONReturnsError(c *C) {
	type T struct {
		A map[int]interface{}
	}
	ti := &T{}
	_, err := client.newRequest(http.MethodGet, "/", ti)
	c.Assert(err, NotNil)
	tp := reflect.TypeOf(ti.A)
	c.Assert(err, FitsTypeOf, &json.UnsupportedTypeError{Type: tp})
}

func (s *ClientSuite) TestNewRequestWithBadURLReturnsError(c *C) {
	_, err := client.newRequest(http.MethodGet, ":", nil)
	c.Assert(err, ErrorMatches, "parse :: missing protocol scheme")
}

// If a nil body is passed to the API, make sure that nil is also
// passed to http.NewRequest.  In most cases, passing an io.Reader that returns
// no content is fine, since there is no difference between an HTTP request
// body that is an empty string versus one that is not set at all.  However in
// certain cases, intermediate systems may treat these differently resulting in
// subtle errors.
func (s *ClientSuite) TestNewRequestWithEmptyBody(c *C) {
	req, err := client.newRequest(http.MethodGet, "/", nil)
	c.Assert(err, IsNil)
	c.Assert(req.Body, IsNil)
}

func (s *ClientSuite) TestDo(c *C) {

	te := CreateTestEvents(1, "some-stream", "localhost:2113", "SomeEventType")
	body := te[0].Data

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodPost)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, body)
	})

	req, _ := client.newRequest(http.MethodPost, "/", nil)
	resp, err := client.do(req, nil)
	c.Assert(err, IsNil)

	want := &Response{
		Response:   resp.Response,
		StatusCode: http.StatusCreated,
		Status:     "201 Created"}

	c.Assert(want, DeepEquals, resp)
}

func (s *ClientSuite) TestErrorResponseContainsCopyOfTheOriginalRequest(c *C) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "")
	})

	req, _ := client.newRequest(http.MethodPost, "/", "[{\"some_field\": 34534}]")

	_, err := client.do(req, nil)

	if e, ok := err.(*ErrBadRequest); ok {
		c.Assert(e.ErrorResponse.Request, DeepEquals, req)
	} else {
		c.FailNow()
	}
}

func (s *ClientSuite) TestErrorResponseContainsStatusCodeAndMessage(c *C) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Response Body")
	})

	req, _ := client.newRequest(http.MethodPost, "/", nil)

	_, err := client.do(req, nil)

	if e, ok := err.(*ErrBadRequest); ok {
		c.Assert(e.ErrorResponse.StatusCode, Equals, http.StatusBadRequest)
		c.Assert(e.ErrorResponse.Status, Equals, "400 Bad Request")
	} else {
		c.FailNow()
	}
}

func (s *ClientSuite) TestNewResponse(c *C) {

	r := http.Response{
		Status:     "201 Created",
		StatusCode: http.StatusCreated,
	}

	resp := newResponse(&r)

	c.Assert(resp.Status, Equals, "201 Created")
	c.Assert(resp.StatusCode, Equals, http.StatusCreated)
}

func (s *ClientSuite) TestGetEvent(c *C) {
	stream := "GetEventStream"
	es := CreateTestEvents(1, stream, server.URL, "SomeEventType")
	ti := Time(time.Now())

	want := CreateTestEventResponse(es[0], &ti)

	er, _ := CreateTestEventAtomResponse(es[0], &ti)
	str := er.PrettyPrint()

	mux.HandleFunc("/streams/some-stream/299", func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Accept")
		want := "application/vnd.eventstore.atom+json"
		c.Assert(got, Equals, want)

		fmt.Fprint(w, str)
	})

	got, _, err := client.GetEvent("/streams/some-stream/299")
	c.Assert(err, IsNil)
	c.Assert(got.PrettyPrint(), Equals, want.PrettyPrint())
}

func (s *ClientSuite) TestGetEventURLs(c *C) {
	es := CreateTestEvents(2, "some-stream", "http://localhost:2113", "EventTypeX")
	f, _ := CreateTestFeed(es, "http://localhost:2113/streams/some-stream/head/backward/2")

	got, err := f.GetEventURLs()
	c.Assert(err, IsNil)
	want := []string{
		"http://localhost:2113/streams/some-stream/1",
		"http://localhost:2113/streams/some-stream/0",
	}
	c.Assert(got, DeepEquals, want)
}

func (s *ClientSuite) TestSoftDeleteStream(c *C) {
	streamName := "foostream"
	mux.HandleFunc("/streams/foostream", func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodDelete)
		h := r.Header.Get("ES-HardDelete")
		c.Assert(h, DeepEquals, "")
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprint(w, "")
	})

	resp, err := client.DeleteStream(streamName, false)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNoContent)
}

func (s *ClientSuite) TestHardDeleteStream(c *C) {
	streamName := "foostream"
	mux.HandleFunc("/streams/foostream", func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodDelete)
		h := r.Header.Get("ES-HardDelete")
		c.Assert(h, DeepEquals, "true")
		w.WriteHeader(http.StatusNoContent)
		fmt.Fprint(w, "")
	})

	resp, err := client.DeleteStream(streamName, true)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNoContent)
}

func (s *ClientSuite) TestDeletingDeletedStreamReturnsDeletedError(c *C) {
	streamName := "foostream"
	mux.HandleFunc("/streams/foostream", func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, http.MethodDelete)
		h := r.Header.Get("ES-HardDelete")
		c.Assert(h, DeepEquals, "true")
		w.WriteHeader(http.StatusGone)
		fmt.Fprint(w, "")
	})

	resp, err := client.DeleteStream(streamName, true)
	c.Assert(err, NotNil)
	c.Assert(typeOf(err), Equals, "ErrDeleted")
	c.Assert(resp.StatusCode, Equals, http.StatusGone)
}

func (s *ClientSuite) TestGetFeedPathForward(c *C) {
	streamName := "foostream"
	direction := "forward"
	version := 0
	pageSize := 10

	want := fmt.Sprintf("/streams/%s/%d/%s/%d", streamName, version, direction, pageSize)

	got, err := client.GetFeedPath(streamName, direction, version, pageSize)
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}

func (s *ClientSuite) TestGetFeedPathBackward(c *C) {
	streamName := "foostream"
	direction := "backward"
	version := 10
	pageSize := 20

	want := fmt.Sprintf("/streams/%s/%d/%s/%d", streamName, version, direction, pageSize)

	got, err := client.GetFeedPath(streamName, direction, version, pageSize)
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}

func (s *ClientSuite) TestGetFeedPathHeadBackward(c *C) {
	streamName := "foostream"
	direction := "backward"
	version := -1
	pageSize := 20

	want := fmt.Sprintf("/streams/%s/%s/%s/%d", streamName, "head", direction, pageSize)

	got, err := client.GetFeedPath(streamName, direction, version, pageSize)
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}

func (s *ClientSuite) TestGetFeedPathInvalidDirection(c *C) {
	streamName := "foostream"
	direction := "somethingwrong"
	version := -1
	pageSize := 20

	want := ""

	got, err := client.GetFeedPath(streamName, direction, version, pageSize)
	c.Assert(err, NotNil)
	c.Assert(got, DeepEquals, want)
	c.Assert(err, DeepEquals, fmt.Errorf("Invalid Direction %s. Allowed values are \"forward\" or \"backward\" \n", direction))
}

func (s *ClientSuite) TestGetFeedPathInvalidDirectionAndVersion(c *C) {
	streamName := "foostream"
	direction := "forward"
	version := -1
	pageSize := 20

	want := ""

	got, err := client.GetFeedPath(streamName, direction, version, pageSize)
	c.Assert(err, NotNil)
	c.Assert(got, DeepEquals, want)
	c.Assert(err, DeepEquals, fmt.Errorf("Invalid Direction (%s) and version (head) combination.\n", direction))
}
