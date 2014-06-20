package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"
)

// Should serve a known static error page if all backend servers are down
// and object isn't in cache/stale.
// NB: ideally this should be a page that we control that has a mechanism
//     to alert us that it has been served.
func TestFailoverErrorPageAllServersDown(t *testing.T) {

	originServer.Stop()
	backupServer1.Stop()
	backupServer2.Stop()

	sourceUrl := fmt.Sprintf("https://%s/?cache-bust=%s", *edgeHost, NewUUID())
	req, err := http.NewRequest("GET", sourceUrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 503 {
		t.Errorf("Invalid StatusCode received. Expected 503, got %d", resp.StatusCode)
	}

	err = StartBackendsInOrder(*edgeHost)
	if err != nil {
		// Bomb out - we do not have a consistent backend, so subsequent tests
		// would fail in unexpected ways.
		log.Fatal(err)
	}

}

// Should serve a known static error page if all backend servers return a
// 5xx response and object isn't in cache/stale.
// NB: ideally this should be a page that we control that has a mechanism
//     to alert us that it has been served.
func TestFailoverErrorPageAllServers5xx(t *testing.T) {
	t.Error("Not implemented")
}

// Should back off requests against origin for a very short period of time
// if origin returns a 5xx response so as not to overwhelm it.
func TestFailoverOrigin5xxBackOff(t *testing.T) {
	t.Error("Not implemented")
}

// Should serve stale object and not hit mirror(s) if origin is down and
// object is beyond TTL but still in cache.
func TestFailoverOriginDownServeStale(t *testing.T) {
	t.Error("Not implemented")
}

// Should serve stale object and not hit mirror(s) if origin returns a 5xx
// response and object is beyond TTL but still in cache.
func TestFailoverOrigin5xxServeStale(t *testing.T) {
	const expectedResponseStale = "going off like stilton"
	const expectedResponseFresh = "as fresh as daisies"

	const respTTL = time.Duration(2 * time.Second)
	const respTTLWithBuffer = 5 * respTTL
	// Allow varnish's beresp.saintmode to expire.
	const waitSaintMode = time.Duration(5 * time.Second)
	headerValue := fmt.Sprintf("max-age=%.0f", respTTL.Seconds())

	backupServer1.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer1.Name
		t.Errorf("Server %s received request and it shouldn't have", name)
		w.Write([]byte(name))
	})
	backupServer2.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer2.Name
		t.Errorf("Server %s received request and it shouldn't have", name)
		w.Write([]byte(name))
	})

	req := NewUniqueEdgeGET(t)

	var expectedBody string
	for requestCount := 1; requestCount < 6; requestCount++ {
		switch requestCount {
		case 1: // Request 1 populates cache.
			expectedBody = expectedResponseStale

			originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", headerValue)
				w.Write([]byte(expectedBody))
			})
		case 2: // Requests 2,3,4 come from stale.
			time.Sleep(respTTLWithBuffer)
			expectedBody = expectedResponseStale

			originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(originServer.Name))
			})
		case 5: // Last request comes directly from origin again.
			time.Sleep(waitSaintMode)
			expectedBody = expectedResponseFresh

			originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(expectedBody))
			})
		}

		resp := RoundTripCheckError(t, req)

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if bodyStr := string(body); bodyStr != expectedBody {
			t.Errorf(
				"Request %d received incorrect response body. Expected %q, got %q",
				requestCount,
				expectedBody,
				bodyStr,
			)
		}
	}
}

// Should fallback to first mirror if origin is down and object is not in
// cache (active or stale).
func TestFailoverOriginDownUseFirstMirror(t *testing.T) {
	t.Error("Not implemented")
}

// Should fallback to first mirror if origin returns 5xx response and object
// is not in cache (active or stale).
func TestFailoverOrigin5xxUseFirstMirror(t *testing.T) {
	expectedBody := "lucky golden ticket"
	expectedStatus := http.StatusOK
	backendsSawRequest := map[string]bool{}

	originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := originServer.Name
		if !backendsSawRequest[name] {
			w.WriteHeader(http.StatusServiceUnavailable)
			backendsSawRequest[name] = true
		} else {
			t.Errorf("Server %s received more than one request", name)
		}
		w.Write([]byte(name))
	})
	backupServer1.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer1.Name
		if !backendsSawRequest[name] {
			w.Write([]byte(expectedBody))
			backendsSawRequest[name] = true
		} else {
			t.Errorf("Server %s received more than one request", name)
			w.Write([]byte(name))
		}
	})
	backupServer2.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer2.Name
		t.Errorf("Server %s received a request and it shouldn't have", name)
		w.Write([]byte(name))
	})

	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)

	if resp.StatusCode != expectedStatus {
		t.Errorf(
			"Received incorrect status code. Expected %d, got %d",
			expectedStatus,
			resp.StatusCode,
		)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bodyStr := string(body); bodyStr != expectedBody {
		t.Errorf(
			"Received incorrect response body. Expected %q, got %q",
			expectedBody,
			bodyStr,
		)
	}
}

// Should fallback to second mirror if both origin and first mirror are
// down.
func TestFailoverOriginDownFirstMirrorDownUseSecondMirror(t *testing.T) {
	t.Error("Not implemented")
}

// Should fallback to second mirror if both origin and first mirror return
// 5xx responses.
func TestFailoverOrigin5xxFirstMirror5xxUseSecondMirror(t *testing.T) {
	expectedBody := "lucky golden ticket"
	expectedStatus := http.StatusOK
	backendsSawRequest := map[string]bool{}

	originServer.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := originServer.Name
		if !backendsSawRequest[name] {
			w.WriteHeader(http.StatusServiceUnavailable)
			backendsSawRequest[name] = true
		} else {
			t.Errorf("Server %s received more than one request", name)
		}
		w.Write([]byte(name))
	})
	backupServer1.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer1.Name
		if !backendsSawRequest[name] {
			w.WriteHeader(http.StatusServiceUnavailable)
			backendsSawRequest[name] = true
		} else {
			t.Errorf("Server %s received more than one request", name)
		}
		w.Write([]byte(name))
	})
	backupServer2.SwitchHandler(func(w http.ResponseWriter, r *http.Request) {
		name := backupServer2.Name
		if !backendsSawRequest[name] {
			w.Write([]byte(expectedBody))
			backendsSawRequest[name] = true
		} else {
			t.Errorf("Server %s received more than one request", name)
			w.Write([]byte(name))
		}
	})

	req := NewUniqueEdgeGET(t)
	resp := RoundTripCheckError(t, req)

	if resp.StatusCode != expectedStatus {
		t.Errorf(
			"Received incorrect status code. Expected %d, got %d",
			expectedStatus,
			resp.StatusCode,
		)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bodyStr := string(body); bodyStr != expectedBody {
		t.Errorf(
			"Received incorrect response body. Expected %q, got %q",
			expectedBody,
			bodyStr,
		)
	}
}

// Should not fallback to mirror if origin returns a 5xx response with a
// No-Fallback header.
func TestFailoverNoFallbackHeader(t *testing.T) {
	t.Error("Not implemented")
}
