package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strconv"
)

type GreylockSearcher struct {
	url		string
	tr		*http.Transport
	client		*http.Client
}

func NewGreylockSearcher(addr string) (Searcher, error) {
	s := &GreylockSearcher {
		url: fmt.Sprintf("http://%s/search", addr),
		tr:		&http.Transport {
			MaxIdleConnsPerHost:		100,
		},
	}
	s.client = &http.Client {
		Transport: s.tr,
	}

	return s, nil
}

func (s *GreylockSearcher) Search(req *SearchRequest) (*SearchResults, error) {
	req_packed, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cound not marshal greylock request: %+v, error: %v", req, err)
	}
	req_body := bytes.NewReader(req_packed)

	http_request, err := http.NewRequest("POST", s.url, req_body)
	if err != nil {
		return nil, fmt.Errorf("cound not create greylock http request, url: %s, error: %v", s.url, err)
	}
	xreq := strconv.Itoa(rand.Int())
	http_request.Header.Set("X-Request", xreq)

	resp, err := s.client.Do(http_request)
	if err != nil {
		return nil, fmt.Errorf("could not send greylock request, url: %s, error: %v", s.url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("returned status: %d, body: '%s'", resp.StatusCode, string(body))
	}

	var res SearchResults
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, fmt.Errorf("could not unpack greylock response: '%s', error: %v", string(body), err)
	}

	return &res, nil
}

func (s *GreylockSearcher) Close() {
}
