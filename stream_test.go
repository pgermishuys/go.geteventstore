// Copyright 2016 Jet Basrawi. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goes

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"

	. "gopkg.in/check.v1"
)

var _ = Suite(&StreamSuite{})

type StreamSuite struct{}

func (s *StreamSuite) SetUpTest(c *C) {
	setup()
}
func (s *StreamSuite) TearDownTest(c *C) {
	teardown()
}

func (s *StreamSuite) TestGetAtomPage(c *C) {

	stream := "some-stream"
	path := fmt.Sprintf("/streams/%s/head/backward/20", stream)
	url := fmt.Sprintf("%s%s", server.URL, path)

	es := createTestEvents(2, stream, server.URL, "EventTypeX")
	f, _ := createTestFeed(es, url)

	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, "GET")
		fmt.Fprint(w, f.PrettyPrint())
		c.Assert(r.Header.Get("Accept"), Equals, "application/atom+xml")
	})

	feed, resp, _ := client.readFeed(url)
	c.Assert(feed.PrettyPrint(), DeepEquals, f.PrettyPrint())
	c.Assert(resp.StatusCode, DeepEquals, http.StatusOK)
}

func (s *StreamSuite) TestUnmarshalFeed(c *C) {
	stream := "unmarshal-feed"
	count := 2

	es := createTestEvents(count, stream, server.URL, "EventTypeX")
	url := fmt.Sprintf("%s/streams/%s/head/backward/%d", server.URL, stream, count)

	wf, _ := createTestFeed(es, url)
	want := wf.PrettyPrint()

	gf, err := unmarshalFeed(strings.NewReader(want))
	c.Assert(err, IsNil)
	got := gf.PrettyPrint()

	c.Assert(got, DeepEquals, want)
}

func (s *StreamSuite) TestRunServer(c *C) {
	stream := "astream"
	es := createTestEvents(100, stream, server.URL, "EventTypeA", "EventTypeB")

	setupSimulator(es, nil)

	_, _, err := client.ReadFeedBackward(stream, nil, nil)
	c.Assert(err, IsNil)
}

func (s *StreamSuite) TestReadFeedBackwardError(c *C) {
	stream := "ABigStream"
	errWant := errors.New("Stream Does Not Exist")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, errWant.Error(), http.StatusNotFound)
	})

	_, resp, err := client.ReadFeedBackward(stream, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNotFound)
}

func (s *StreamSuite) TestReadFeedBackwardFromVersionAll(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")

	setupSimulator(es, nil)

	ver := &StreamVersion{Number: 100}

	evs, _, err := client.ReadFeedBackward(stream, ver, nil)
	c.Assert(err, IsNil)
	nex := ver.Number + 1
	c.Assert(evs, HasLen, nex)
	c.Assert(evs[0].Event.EventNumber, Equals, ver.Number)
	c.Assert(evs[len(evs)-1].Event.EventNumber, Equals, 0)
	for k, v := range evs {
		ex := (nex - 1) - k
		c.Assert(v.Event.EventNumber, Equals, ex)
	}
}

func (s *StreamSuite) TestReadFeedBackwardAll(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")

	setupSimulator(es, nil)

	evs, _, err := client.ReadFeedBackward(stream, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(evs, HasLen, ne)
	c.Assert(evs[0].Event.EventNumber, Equals, ne-1)
	c.Assert(evs[len(evs)-1].Event.EventNumber, Equals, 0)
	for k, v := range evs {
		ex := (ne - 1) - k
		c.Assert(v.Event.EventNumber, Equals, ex)
	}
}

func (s *StreamSuite) TestReadFeedForwardError(c *C) {
	stream := "ABigStream"
	errWant := errors.New("Stream Does Not Exist")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, errWant.Error(), http.StatusNotFound)
	})

	_, resp, err := client.ReadFeedForward(stream, nil, nil)
	c.Assert(err, NotNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNotFound)
}

func (s *StreamSuite) TestReadFeedBackwardFromVersionWithTake(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")

	setupSimulator(es, nil)

	ver := &StreamVersion{Number: 667}
	take := &Take{Number: 14}

	evs, _, err := client.ReadFeedBackward(stream, ver, take)
	c.Assert(err, IsNil)
	nex := take.Number
	c.Assert(evs, HasLen, nex)

	lstn := ver.Number - (take.Number - 1)
	c.Assert(evs[0].Event.EventNumber, Equals, ver.Number)
	c.Assert(evs[len(evs)-1].Event.EventNumber, Equals, lstn)
	for k, v := range evs {
		ex := ver.Number - k
		c.Assert(v.Event.EventNumber, Equals, ex)
	}
}

func (s *StreamSuite) TestReadFeedBackwardFromVersionWithTakeOutOfRangeUnder(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")

	setupSimulator(es, nil)

	ver := &StreamVersion{Number: 49}
	take := &Take{Number: 59}

	evs, _, err := client.ReadFeedBackward(stream, ver, take)
	c.Assert(err, IsNil)

	nex := ver.Number + 1
	c.Assert(evs, HasLen, nex)

	lstn := 0
	c.Assert(evs[0].Event.EventNumber, Equals, ver.Number)
	c.Assert(evs[len(evs)-1].Event.EventNumber, Equals, lstn)
	for k, v := range evs {
		ex := ver.Number - k
		c.Assert(v.Event.EventNumber, Equals, ex)
	}
}

//Try to get versions past head of stream that do not yet exist
//this use case is used to poll head of stream waiting for new events
func (s *StreamSuite) TestReadFeedForwardTail(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")
	setupSimulator(es, nil)
	ver := &StreamVersion{Number: 1000}

	evs, _, err := client.ReadFeedForward(stream, ver, nil)

	c.Assert(err, IsNil)
	c.Assert(evs, HasLen, 0)
}

func (s *StreamSuite) TestGetFeedURLInvalidVersion(c *C) {
	stream := "ABigStream"
	ver := &StreamVersion{Number: -1}

	_, err := getFeedURL(stream, "forward", ver, nil)
	c.Assert(err, FitsTypeOf, invalidVersionError(ver.Number))
}

func (s *StreamSuite) TestReadFeedForwardAll(c *C) {
	stream := "ABigStream"
	ne := 1000
	es := createTestEvents(ne, stream, server.URL, "EventTypeX")

	setupSimulator(es, nil)

	evs, _, err := client.ReadFeedForward(stream, nil, nil)
	c.Assert(err, IsNil)
	c.Assert(evs, HasLen, ne)
	c.Assert(evs[0].Event.EventNumber, Equals, 0)
	c.Assert(evs[len(evs)-1].Event.EventNumber, Equals, ne-1)
	for k, v := range evs {
		c.Assert(v.Event.EventNumber, Equals, k)
	}
}

func (s *StreamSuite) TestGetFeedURLForwardLowTake(c *C) {
	want := "/streams/some-stream/0/forward/10"
	got, _ := getFeedURL("some-stream", "forward", nil, &Take{Number: 10})
	c.Assert(got, Equals, want)
}

func (s *StreamSuite) TestGetFeedURLBackwardLowTake(c *C) {
	want := "/streams/some-stream/head/backward/15"
	got, _ := getFeedURL("some-stream", "backward", nil, &Take{Number: 15})
	c.Assert(got, Equals, want)
}

func (s *StreamSuite) TestGetFeedURLInvalidDirection(c *C) {
	want := errors.New("Invalid Direction")
	_, got := getFeedURL("some-stream", "nonesense", nil, nil)
	c.Assert(got, DeepEquals, want)
}

func (s *StreamSuite) TestGetFeedURLBackwardNilAll(c *C) {
	want := "/streams/some-stream/head/backward/100"
	got, _ := getFeedURL("some-stream", "", nil, nil)
	c.Assert(got, Equals, want)
}

func (s *StreamSuite) TestGetFeedURLForwardNilAll(c *C) {
	want := "/streams/some-stream/0/forward/100"
	got, _ := getFeedURL("some-stream", "forward", nil, nil)
	c.Assert(got, Equals, want)
}

func (s *StreamSuite) TestGetFeedURLForwardVersioned(c *C) {
	want := "/streams/some-stream/15/forward/100"
	got, _ := getFeedURL("some-stream", "forward", &StreamVersion{Number: 15}, nil)
	c.Assert(got, Equals, want)
}

func (s *StreamSuite) TestGetMetaReturnsNilWhenStreamMetaDataIsEmpty(c *C) {
	stream := "Some-Stream"
	es := createTestEvents(10, stream, server.URL, "EventTypeX")
	setupSimulator(es, nil)

	got, resp, err := client.GetStreamMetaData(stream)

	c.Assert(err, IsNil)
	c.Assert(got, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
}

func (s *StreamSuite) TestGetMetaData(c *C) {
	d := fmt.Sprintf("{ \"foo\" : %d }", rand.Intn(9999))
	raw := json.RawMessage(d)
	stream := "Some-Stream"
	es := createTestEvents(10, stream, server.URL, "EventTypeX")
	m := createTestEvent(stream, server.URL, "metadata", 10, &raw, nil)
	want, _ := createTestEventResponse(m, nil)
	setupSimulator(es, m)

	got, _, _ := client.GetStreamMetaData(stream)

	c.Assert(got.PrettyPrint(), Equals, want.PrettyPrint())
}

func (s *StreamSuite) TestAppendStreamMetadata(c *C) {
	eventType := "MetaData"
	stream := "Some-Stream"

	// Before updating the metadata, the method needs to get the MetaData url
	// According to the docs, the eventstore team reserve the right to change
	// the metadata url.
	fURL := fmt.Sprintf("/streams/%s/head/backward/100", stream)
	fullURL := fmt.Sprintf("%s%s", server.URL, fURL)
	mux.HandleFunc(fURL, func(w http.ResponseWriter, r *http.Request) {
		es := createTestEvents(1, stream, server.URL, eventType)
		f, _ := createTestFeed(es, fullURL)
		fmt.Fprint(w, f.PrettyPrint())
	})

	meta := fmt.Sprintf("{\"baz\":\"boo\"}")
	want := json.RawMessage(meta)

	url := fmt.Sprintf("/streams/%s/metadata", stream)

	mux.HandleFunc(url, func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, Equals, "POST")

		var got json.RawMessage
		ev := &Event{Data: &got}
		err := json.NewDecoder(r.Body).Decode(ev)
		c.Assert(err, IsNil)
		c.Assert(got, DeepEquals, want)

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "")
	})

	resp, err := client.UpdateStreamMetaData(stream, &want)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusMessage, Equals, "201 Created")
	c.Assert(resp.StatusCode, Equals, http.StatusCreated)
}