package integration

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestDeleteByID(t *testing.T) {
	env := newTestEnv(t)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})
	resp := uploadZip(t, env, zipData)
	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()
	id := body["id"].(string)

	waitForStatus(t, env, id, "completed", 5*time.Second)

	req, _ := http.NewRequest(http.MethodDelete, env.server.URL+"/reports/"+id, nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	getResp, err := http.Get(env.server.URL + "/reports/" + id + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestDeleteAllReports(t *testing.T) {
	env := newTestEnv(t)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})

	ids := make([]string, 3)
	for i := range ids {
		resp := uploadZip(t, env, zipData)
		var body map[string]interface{}
		decodeJSON(t, resp.Body, &body)
		resp.Body.Close()
		ids[i] = body["id"].(string)
	}

	for _, id := range ids {
		waitForStatus(t, env, id, "completed", 5*time.Second)
	}

	req, _ := http.NewRequest(http.MethodDelete, env.server.URL+"/reports", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	for _, id := range ids {
		getResp, err := http.Get(env.server.URL + "/reports/" + id + "/status")
		if err != nil {
			t.Fatal(err)
		}
		getResp.Body.Close()
		if getResp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after delete all, got %d for %s", getResp.StatusCode, id)
		}
	}
}

func TestCompletedButMissingFiles(t *testing.T) {
	env := newTestEnv(t)

	zipData, _ := buildZip(map[string]string{"index.html": "hi"})
	resp := uploadZip(t, env, zipData)
	var body map[string]interface{}
	decodeJSON(t, resp.Body, &body)
	resp.Body.Close()
	id := body["id"].(string)

	waitForStatus(t, env, id, "completed", 5*time.Second)

	// Remove the output directory to simulate missing files.
	outDir := env.dataDir + "/reports/" + id[:2] + "/" + id
	if err := os.RemoveAll(outDir); err != nil {
		t.Fatal(err)
	}

	// GET should return failure page.
	getResp, err := http.Get(env.server.URL + "/reports/" + id + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 failure page, got %d", getResp.StatusCode)
	}
}
