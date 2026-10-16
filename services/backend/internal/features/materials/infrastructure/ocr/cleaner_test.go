package ocr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCleaningUsesJointExtractionAndMaterialIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/topics/extract" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["material_id"] != "lecture" || body["markdown"] != "Текст" {
			t.Errorf("wrong body: %+v", body)
		}
		_, _ = w.Write([]byte(`{"markdown":"Текст","report":{"version":"material-cleaning-v1","topics_version":"topics-v1","topics":[]}}`))
	}))
	defer server.Close()
	result, err := New(server.URL, time.Second).Clean(context.Background(), "Текст", "lecture")
	if err != nil || result.Markdown != "Текст" {
		t.Fatalf("result: %+v %v", result, err)
	}
}
