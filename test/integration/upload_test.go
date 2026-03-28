package integration

import (
	"net/http"
	"testing"
	"time"
)

func TestHappyPath(t *testing.T) {
	env := newTestEnv(t)

	zipData, err := buildZip(map[string]string{
		"index.html": "<html>report</html>",
		"data/foo":   "trace",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp := uploadZip(t, env, zipData)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()

	id, ok := body["id"].(string)
	if !ok || id == "" {
		t.Fatal("missing id in response")
	}
	if body["status"] != "queued" {
		t.Fatalf("expected queued, got %v", body["status"])
	}

	waitForStatus(t, env, id, "completed", 5*time.Second)

	getResp, err := http.Get(env.server.URL + "/reports/" + id + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 serving report, got %d", getResp.StatusCode)
	}
}

func TestOversizedUpload(t *testing.T) {
	env := newTestEnvWithMaxBytes(t, 10)

	zipData, _ := buildZip(map[string]string{"index.html": "hello world this is a large content"})

	resp := uploadZip(t, env, zipData)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}
}

func TestInvalidZip(t *testing.T) {
	env := newTestEnv(t)

	resp := uploadRaw(t, env, []byte("not a zip file"))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUnknownID(t *testing.T) {
	env := newTestEnv(t)

	resp, err := http.Get(env.server.URL + "/reports/00000000-0000-0000-0000-000000000000/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestProcessingPage(t *testing.T) {
	env := newTestEnvWithWorkers(t, 0)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})
	resp := uploadZip(t, env, zipData)
	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()
	id := body["id"].(string)

	getResp, err := http.Get(env.server.URL + "/reports/" + id + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 processing page, got %d", getResp.StatusCode)
	}
}

func TestTrailingSlashRedirect(t *testing.T) {
	env := newTestEnv(t)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})
	resp := uploadZip(t, env, zipData)
	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()
	id := body["id"].(string)

	waitForStatus(t, env, id, "completed", 5*time.Second)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	redirResp, err := client.Get(env.server.URL + "/reports/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer redirResp.Body.Close()
	if redirResp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected 301 redirect, got %d", redirResp.StatusCode)
	}
}
