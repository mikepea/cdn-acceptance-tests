package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Test that useful common cache-related parameters are sent to the
// client by this CDN provider.

// Should propagate an Age header from origin and then increment it for the
// time it's in cache.
func TestRespHeaderAge(t *testing.T) {
	ResetBackends(backendsByPriority)

	const originAgeInSeconds = 100
	const secondsToWaitBetweenRequests = 5
	const expectedAgeInSeconds = originAgeInSeconds + secondsToWaitBetweenRequests
	requestReceivedCount := 0

	originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		if requestReceivedCount == 0 {
			w.Header().Set("Cache-Control", "max-age=1800, public")
			w.Header().Set("Age", fmt.Sprintf("%d", originAgeInSeconds))
			w.Write([]byte("cacheable request"))
		} else {
			t.Error("Unexpected subsequent request received at Origin")
		}
		requestReceivedCount++
	})

	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Edge returned an unexpected status: %q", resp.Status)
	}

	// wait a little bit. Edge should update the Age header, we know Origin will not
	time.Sleep(time.Duration(secondsToWaitBetweenRequests) * time.Second)
	resp = RoundTripCheckError(t, req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Edge returned an unexpected status: %q", resp.Status)
	}

	edgeAgeHeader := resp.Header.Get("Age")
	if edgeAgeHeader == "" {
		t.Fatal("Age Header is not set")
	}

	edgeAgeInSeconds, convErr := strconv.Atoi(edgeAgeHeader)
	if convErr != nil {
		t.Fatal(convErr)
	}

	if edgeAgeInSeconds != expectedAgeInSeconds {
		t.Errorf(
			"Age header from Edge is not as expected. Got %q, expected '%d'",
			edgeAgeHeader,
			expectedAgeInSeconds,
		)
	}
}

// Should set an X-Cache header containing HIT/MISS from 'origin, itself'
func TestRespHeaderXCacheAppend(t *testing.T) {
	ResetBackends(backendsByPriority)

	const originXCache = "HIT"

	var (
		xCache         string
		expectedXCache string
	)

	originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cache", originXCache)
	})

	// Get first request, will come from origin, cannot be cached - hence cache MISS
	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)
	defer resp.Body.Close()

	xCache = resp.Header.Get("X-Cache")
	expectedXCache = fmt.Sprintf("%s, MISS", originXCache)
	if xCache != expectedXCache {
		t.Errorf(
			"X-Cache on initial hit is wrong: expected %q, got %q",
			expectedXCache,
			xCache,
		)
	}

}

// Should set an X-Cache header containing only MISS if origin does not set an X-Cache Header'
func TestRespHeaderXCacheCreate(t *testing.T) {
	ResetBackends(backendsByPriority)

	const expectedXCache = "MISS"

	var (
		xCache string
	)

	// Get first request, will come from origin, cannot be cached - hence cache MISS
	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)
	defer resp.Body.Close()

	xCache = resp.Header.Get("X-Cache")
	if xCache != expectedXCache {
		t.Errorf(
			"X-Cache on initial hit is wrong: expected %q, got %q",
			expectedXCache,
			xCache,
		)
	}

}

// Should set an 'Served-By' header giving information on the edge node and location served from.
func TestRespHeaderServedBy(t *testing.T) {
	ResetBackends(backendsByPriority)

	var expectedServedByRegexp *regexp.Regexp
	var headerName string

	switch {
	case vendorCloudflare:
		headerName = "CF-RAY"
		expectedServedByRegexp = regexp.MustCompile("^[a-z0-9]{16}-[A-Z]{3}$")
	case vendorFastly:
		headerName = "X-Served-By"
		expectedServedByRegexp = regexp.MustCompile("^cache-[a-z0-9]+-[A-Z]{3}$")
	default:
		t.Fatal(notImplementedForVendor)
	}

	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)
	defer resp.Body.Close()

	actualHeader := resp.Header.Get(headerName)

	if actualHeader == "" {
		t.Error(headerName + " header has not been set by Edge")
	}

	if expectedServedByRegexp.FindString(actualHeader) != actualHeader {
		t.Errorf("%s is not as expected: got %q", headerName, actualHeader)
	}

}

// Should set an X-Cache-Hits header containing hit count for this object,
// from the Edge AND the Origin, assuming Origin sets one.
// This is in the format "{origin-hit-count}, {edge-hit-count}"
func TestRespHeaderXCacheHitsAppend(t *testing.T) {
	ResetBackends(backendsByPriority)

	const originXCacheHits = "53"

	var (
		xCacheHits         string
		expectedXCacheHits string
	)

	uuid := NewUUID()

	originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == fmt.Sprintf("/%s", uuid) {
			w.Header().Set("X-Cache-Hits", originXCacheHits)
		}
	})

	sourceUrl := fmt.Sprintf("https://%s/%s", *edgeHost, uuid)

	// Get first request, will come from origin. Edge Hit Count 0
	req, _ := http.NewRequest("GET", sourceUrl, nil)
	resp := RoundTripCheckError(t, req)
	defer resp.Body.Close()

	xCacheHits = resp.Header.Get("X-Cache-Hits")
	expectedXCacheHits = fmt.Sprintf("%s, 0", originXCacheHits)
	if xCacheHits != expectedXCacheHits {
		t.Errorf(
			"X-Cache-Hits on initial hit is wrong: expected %q, got %q",
			expectedXCacheHits,
			xCacheHits,
		)
	}

	// Get request again. Should come from Edge now, hit count 1
	resp = RoundTripCheckError(t, req)
	defer resp.Body.Close()

	xCacheHits = resp.Header.Get("X-Cache-Hits")
	expectedXCacheHits = fmt.Sprintf("%s, 1", originXCacheHits)
	if xCacheHits != expectedXCacheHits {
		t.Errorf(
			"X-Cache-Hits on second hit is wrong: expected %q, got %q",
			expectedXCacheHits,
			xCacheHits,
		)
	}
}
