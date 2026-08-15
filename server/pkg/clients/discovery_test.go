package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %s, want /v1/models", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("missing bearer auth")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[
			{"id":"deepseek-chat","context_window":65536,"max_output_tokens":8192},
			{"id":"deepseek-reasoner","display_name":"DeepSeek Reasoner","context_length":131072},
			{"name":"row without id"},
			{"id":"  "}
		]}`))
	}))
	defer srv.Close()

	models, err := ListModels(context.Background(), srv.URL+"/v1/", "sk-test")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2 (rows without usable id skipped)", len(models))
	}
	if models[0].ID != "deepseek-chat" || models[0].ContextLength != 65536 || models[0].MaxTokens != 8192 {
		t.Errorf("models[0] = %+v", models[0])
	}
	if models[1].Name != "DeepSeek Reasoner" || models[1].ContextLength != 131072 {
		t.Errorf("models[1] = %+v", models[1])
	}
}

func TestListModelsErrors(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthorized.Close()
	if _, err := ListModels(context.Background(), unauthorized.URL, "bad-key"); err == nil || !strings.Contains(err.Error(), "401; check the API key") {
		t.Errorf("401 error = %v", err)
	}

	notListing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"object":"list"}`))
	}))
	defer notListing.Close()
	if _, err := ListModels(context.Background(), notListing.URL, ""); err == nil {
		t.Error("expected error for reply without data array")
	}
}
