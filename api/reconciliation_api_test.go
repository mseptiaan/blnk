/*
Copyright 2024 Blnk Finance Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	model2 "github.com/blnkfinance/blnk/api/model"
	"github.com/blnkfinance/blnk/config"
	"github.com/blnkfinance/blnk/internal/request"
	"github.com/stretchr/testify/assert"
)

func TestStartReconciliation(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Missing required fields", func(t *testing.T) {
		payload := struct {
			UploadID string `json:"upload_id"`
			Strategy string `json:"strategy"`
		}{
			UploadID: "",
			Strategy: "",
		}
		payloadBytes, _ := request.ToJsonReq(&payload)
		req := httptest.NewRequest("POST", "/reconciliation/start", payloadBytes)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/reconciliation/start", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestInstantReconciliation(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Missing external transactions", func(t *testing.T) {
		payload := struct {
			ExternalTransactions []interface{} `json:"external_transactions"`
			Strategy             string        `json:"strategy"`
			MatchingRuleIDs      []string      `json:"matching_rule_ids"`
		}{
			ExternalTransactions: []interface{}{},
			Strategy:             "one_to_one",
			MatchingRuleIDs:      []string{"mr_test"},
		}
		payloadBytes, _ := request.ToJsonReq(&payload)
		req := httptest.NewRequest("POST", "/reconciliation/start-instant", payloadBytes)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/reconciliation/start-instant", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Too many external transactions", func(t *testing.T) {
		txns := make([]map[string]interface{}, model2.MaxInstantReconciliationItems+1)
		for i := range txns {
			txns[i] = map[string]interface{}{"id": fmt.Sprintf("ext_%d", i), "amount": 1, "reference": fmt.Sprintf("r%d", i), "currency": "USD"}
		}
		payload := map[string]interface{}{
			"external_transactions": txns,
			"strategy":              "one_to_one",
			"matching_rule_ids":     []string{"mr_test"},
		}
		payloadBytes, _ := request.ToJsonReq(&payload)
		req := httptest.NewRequest("POST", "/reconciliation/start-instant", payloadBytes)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "too many external_transactions")
	})
}

func TestGetReconciliation(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Reconciliation not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/reconciliation/rec_nonexistent", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}

func TestCreateMatchingRule(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/reconciliation/matching-rules", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestUpdateMatchingRule(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Missing rule ID", func(t *testing.T) {
		payload := struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{
			Name:        "Updated Rule",
			Description: "Updated description",
		}
		payloadBytes, _ := request.ToJsonReq(&payload)
		req := httptest.NewRequest("PUT", "/reconciliation/matching-rules/", payloadBytes)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/reconciliation/matching-rules/mr_test", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestDeleteMatchingRule(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Missing rule ID", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/reconciliation/matching-rules/", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("Delete non-existent rule", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/reconciliation/matching-rules/mr_nonexistent", nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}

func uploadMultipart(t *testing.T, router *gin.Engine, fieldName, fileName, source string, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if source != "" {
		if err := writer.WriteField("source", source); err != nil {
			t.Fatalf("Failed to write source field: %v", err)
		}
	}
	if fieldName != "" {
		part, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			t.Fatalf("Failed to create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/reconciliation/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestUploadExternalData(t *testing.T) {
	router, _, err := setupRouter()
	if err != nil {
		t.Fatalf("Failed to setup router: %v", err)
	}

	t.Run("Valid CSV upload", func(t *testing.T) {
		id1, id2 := "ext_"+gofakeit.UUID(), "ext_"+gofakeit.UUID()
		csvContent := []byte("ID,Amount,Currency,Reference,Description,Date\n" +
			id1 + ",100.50,USD,ref_" + gofakeit.UUID() + ",test row,2024-01-01T10:00:00Z\n" +
			id2 + ",200.00,USD,ref_" + gofakeit.UUID() + ",test row two,2024-01-02T10:00:00Z\n")
		resp := uploadMultipart(t, router, "file", "data.csv", "bank-test", csvContent)
		assert.Equal(t, http.StatusOK, resp.Code)

		var response map[string]interface{}
		if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Contains(t, response["upload_id"], "upload_")
		assert.Equal(t, float64(2), response["record_count"])
		assert.Equal(t, "bank-test", response["source"])
	})

	t.Run("Valid JSON upload", func(t *testing.T) {
		jsonContent := []byte(fmt.Sprintf(`[{"id": "ext_%s", "amount": 300, "currency": "USD", "reference": "ref_%s", "description": "json row", "date": "2024-01-03T10:00:00Z"}]`, gofakeit.UUID(), gofakeit.UUID()))
		resp := uploadMultipart(t, router, "file", "data.json", "bank-test", jsonContent)
		assert.Equal(t, http.StatusOK, resp.Code)

		var response map[string]interface{}
		if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Equal(t, float64(1), response["record_count"])
	})

	t.Run("Missing file part", func(t *testing.T) {
		resp := uploadMultipart(t, router, "", "", "bank-test", nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "File upload failed")
	})

	t.Run("CSV missing required columns", func(t *testing.T) {
		csvContent := []byte("Foo,Bar\n1,2\n")
		resp := uploadMultipart(t, router, "file", "data.csv", "bank-test", csvContent)
		assert.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.Contains(t, resp.Body.String(), "Failed to process upload")
	})
}

func TestUploadExternalData_RejectsOversizedBody(t *testing.T) {
	// Cap uploads at 1 MB, then send a body larger than that: the request
	// must be rejected with 413 before the file is processed.
	router, _, _ := setupRouterWithConfig(t, func(c *config.Configuration) {
		c.Server.MaxUploadSizeMB = 1
	})

	oversized := bytes.Repeat([]byte("a"), 2*1024*1024) // 2 MB
	resp := uploadMultipart(t, router, "file", "big.csv", "bank-test", oversized)

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Code)
	assert.Contains(t, resp.Body.String(), "exceeds the maximum allowed size")
}

func uploadURL(t *testing.T, router *gin.Engine, rawURL, source string) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if source != "" {
		if err := writer.WriteField("source", source); err != nil {
			t.Fatalf("Failed to write source field: %v", err)
		}
	}
	if rawURL != "" {
		if err := writer.WriteField("url", rawURL); err != nil {
			t.Fatalf("Failed to write url field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/reconciliation/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestUploadExternalData_URL(t *testing.T) {
	t.Run("Valid CSV via URL", func(t *testing.T) {
		id1, id2 := "ext_"+gofakeit.UUID(), "ext_"+gofakeit.UUID()
		csvContent := "ID,Amount,Currency,Reference,Description,Date\n" +
			id1 + ",100.50,USD,ref_" + gofakeit.UUID() + ",test row,2024-01-01T10:00:00Z\n" +
			id2 + ",200.00,USD,ref_" + gofakeit.UUID() + ",test row two,2024-01-02T10:00:00Z\n"

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprint(w, csvContent)
		}))
		defer ts.Close()

		router, _, err := setupRouter()
		if err != nil {
			t.Fatalf("Failed to setup router: %v", err)
		}

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-url-test")
		assert.Equal(t, http.StatusOK, resp.Code)

		var response map[string]interface{}
		if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Contains(t, response["upload_id"], "upload_")
		assert.Equal(t, float64(2), response["record_count"])
		assert.Equal(t, "bank-url-test", response["source"])
	})

	t.Run("Invalid URL scheme", func(t *testing.T) {
		router, _, err := setupRouter()
		if err != nil {
			t.Fatalf("Failed to setup router: %v", err)
		}

		resp := uploadURL(t, router, "ftp://example.com/data.csv", "bank-test")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "http or https")
	})

	t.Run("URL server returns error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		router, _, err := setupRouter()
		if err != nil {
			t.Fatalf("Failed to setup router: %v", err)
		}

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-test")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "URL returned an error")
	})

	t.Run("File wins over URL when both present", func(t *testing.T) {
		id1 := "ext_" + gofakeit.UUID()
		csvContent := []byte("ID,Amount,Currency,Reference,Description,Date\n" +
			id1 + ",100.50,USD,ref_" + gofakeit.UUID() + ",test row,2024-01-01T10:00:00Z\n")

		// This server should NOT be hit because file takes priority.
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("URL server should not be called when file is present")
		}))
		defer ts.Close()

		router, _, err := setupRouter()
		if err != nil {
			t.Fatalf("Failed to setup router: %v", err)
		}

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		_ = writer.WriteField("source", "bank-file")
		_ = writer.WriteField("url", ts.URL+"/should-not-be-used.csv")
		part, _ := writer.CreateFormFile("file", "data.csv")
		_, _ = part.Write(csvContent)
		writer.Close()

		req := httptest.NewRequest("POST", "/reconciliation/upload", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)

		var response map[string]interface{}
		if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Equal(t, float64(1), response["record_count"])
		assert.Equal(t, "bank-file", response["source"])
	})

	t.Run("Domain not in whitelist", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "ID,Amount,Currency,Reference,Description,Date\n")
		}))
		defer ts.Close()

		router, _, _ := setupRouterWithConfig(t, func(c *config.Configuration) {
			c.Server.UploadWhitelist = "trusted.example.com,other.example.com"
		})

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-test")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "not whitelisted")
	})

	t.Run("Domain in whitelist allowed", func(t *testing.T) {
		id1 := "ext_" + gofakeit.UUID()
		csvContent := "ID,Amount,Currency,Reference,Description,Date\n" +
			id1 + ",100.50,USD,ref_" + gofakeit.UUID() + ",test row,2024-01-01T10:00:00Z\n"

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprint(w, csvContent)
		}))
		defer ts.Close()

		// Extract host from test server URL for whitelist.
		parsedURL, _ := url.Parse(ts.URL)
		router, _, _ := setupRouterWithConfig(t, func(c *config.Configuration) {
			c.Server.UploadWhitelist = parsedURL.Hostname()
		})

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-test")
		assert.Equal(t, http.StatusOK, resp.Code)

		var response map[string]interface{}
		if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		assert.Equal(t, float64(1), response["record_count"])
	})

	t.Run("SSRF blocks private IP", func(t *testing.T) {
		client := safeHTTPClient(5*time.Second, false)
		_, err := client.Get("http://127.0.0.1/data.csv")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "private IP")
	})

	t.Run("Content-Length rejects oversized payload", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "999999999")
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		router, _, _ := setupRouterWithConfig(t, func(c *config.Configuration) {
			c.Server.MaxUploadSizeMB = 1
		})

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-test")
		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Code)
		assert.Contains(t, resp.Body.String(), "exceeds the maximum allowed size")
	})

	t.Run("Download exceeding size limit returns error", func(t *testing.T) {
		bigContent := strings.Repeat("x", 2*1024*1024)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprint(w, bigContent)
		}))
		defer ts.Close()

		router, _, _ := setupRouterWithConfig(t, func(c *config.Configuration) {
			c.Server.MaxUploadSizeMB = 1
		})

		resp := uploadURL(t, router, ts.URL+"/data.csv", "bank-test")
		assert.Equal(t, http.StatusBadRequest, resp.Code)
		assert.Contains(t, resp.Body.String(), "exceeds the maximum allowed size")
	})
}
