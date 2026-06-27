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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	model2 "github.com/blnkfinance/blnk/api/model"
	"github.com/blnkfinance/blnk/config"
	"github.com/blnkfinance/blnk/internal/apierror"
	"github.com/blnkfinance/blnk/model"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// UploadExternalData handles the upload of external transaction data.
// It receives the file and source details from the request, processes the upload,
// and returns an upload ID along with the record count and source information.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the file upload fails.
// - 500 Internal Server Error: If there is an error processing the upload.
// - 200 OK: If the upload is successful.
func (a Api) UploadExternalData(c *gin.Context) {
	var maxBytes int64
	if cfg, cfgErr := config.Fetch(); cfgErr == nil && cfg.Server.MaxUploadSizeMB > 0 {
		maxBytes = cfg.Server.MaxUploadSizeMB * 1024 * 1024
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	}

	source := c.PostForm("source")

	// Path 1: multipart file upload (existing behaviour, unchanged).
	file, header, fileErr := c.Request.FormFile("file")
	if fileErr == nil {
		defer file.Close()
		fileName := header.Filename
		uploadID, total, err := a.blnk.UploadExternalData(c.Request.Context(), source, file, fileName)
		if err != nil {
			logrus.Error(err)
			respondCode(c, apierror.ErrReconUploadProcessingFailed, "Failed to process upload", nil)
			return
		}
		c.JSON(http.StatusOK, gin.H{"upload_id": uploadID, "record_count": total, "source": source})
		return
	}

	// Check for oversized body before falling through to URL path.
	if strings.Contains(fileErr.Error(), "request body too large") {
		respondCode(c, apierror.ErrGenPayloadTooLarge, "upload exceeds the maximum allowed size", nil)
		return
	}

	// Path 2: URL-based download.
	rawURL := c.PostForm("url")
	if rawURL == "" {
		// Try JSON body: {"url": "...", "source": "..."}
		var body struct {
			URL    string `json:"url"`
			Source string `json:"source"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err == nil && body.URL != "" {
			rawURL = body.URL
			if source == "" {
				source = body.Source
			}
		}
	}

	if rawURL == "" {
		respondCode(c, apierror.ErrReconUploadFailed, "File upload failed", nil)
		return
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		respondCode(c, apierror.ErrReconURLInvalidScheme, "URL must use http or https scheme", nil)
		return
	}

	// Domain whitelist check.
	if cfg, cfgErr := config.Fetch(); cfgErr == nil && cfg.Server.UploadWhitelist != "" {
		allowed := false
		host := strings.ToLower(parsedURL.Hostname())
		for _, domain := range strings.Split(cfg.Server.UploadWhitelist, ",") {
			domain = strings.TrimSpace(strings.ToLower(domain))
			if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
				allowed = true
				break
			}
		}
		if !allowed {
			respondCode(c, apierror.ErrReconURLNotWhitelisted, "URL domain is not whitelisted", nil)
			return
		}
	}

	fileName := path.Base(parsedURL.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "download"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		respondCode(c, apierror.ErrReconURLFetchFailed, "Failed to fetch URL", nil)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logrus.Error(err)
		respondCode(c, apierror.ErrReconURLFetchFailed, "Failed to fetch URL", nil)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respondCode(c, apierror.ErrReconURLFetchFailed, "URL returned an error", nil)
		return
	}

	var reader io.Reader = resp.Body
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes)
	}

	uploadID, total, err := a.blnk.UploadExternalData(c.Request.Context(), source, reader, fileName)
	if err != nil {
		logrus.Error(err)
		respondCode(c, apierror.ErrReconUploadProcessingFailed, "Failed to process upload", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"upload_id": uploadID, "record_count": total, "source": source})
}

// StartReconciliation initiates a new reconciliation process based on the provided parameters.
// It starts the reconciliation process and returns the reconciliation ID.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the request body is invalid or required fields are missing.
// - 500 Internal Server Error: If there is an error starting the reconciliation process.
// - 200 OK: If the reconciliation process is successfully started.
func (a Api) StartReconciliation(c *gin.Context) {
	var req struct {
		UploadID         string   `json:"upload_id" binding:"required"`
		Strategy         string   `json:"strategy" binding:"required"`
		GroupingCriteria string   `json:"grouping_criteria"`
		DryRun           bool     `json:"dry_run"`
		MatchingRuleIDs  []string `json:"matching_rule_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respondCode(c, apierror.ErrGenMalformedRequest, err.Error(), nil)
		return
	}
	if len(req.MatchingRuleIDs) == 0 {
		respondCode(c, apierror.ErrReconMatchingRulesRequired, "matching_rule_ids is required", nil)
		return
	}

	reconciliationID, err := a.blnk.StartReconciliation(c.Request.Context(), req.UploadID, req.Strategy, req.GroupingCriteria, req.MatchingRuleIDs, req.DryRun)
	if err != nil {
		logrus.Error(err)
		respondError(c, err, withDefault(apierror.ErrReconStartFailed), withFallbackMessage("Failed to start reconciliation"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"reconciliation_id": reconciliationID})
}

// InstantReconciliation initiates a reconciliation process with externally provided transactions
// without requiring a prior file upload. It processes the transactions directly and returns
// the reconciliation ID.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the request body is invalid or required fields are missing.
// - 500 Internal Server Error: If there is an error starting the reconciliation process.
// - 200 OK: If the reconciliation process is successfully started.
func (a Api) InstantReconciliation(c *gin.Context) {
	var req struct {
		ExternalTransactions []model.ExternalTransaction `json:"external_transactions" binding:"required"`
		Strategy             string                      `json:"strategy" binding:"required"`
		GroupingCriteria     string                      `json:"grouping_criteria"`
		DryRun               bool                        `json:"dry_run"`
		MatchingRuleIDs      []string                    `json:"matching_rule_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respondCode(c, apierror.ErrGenMalformedRequest, err.Error(), nil)
		return
	}
	if len(req.ExternalTransactions) == 0 {
		respondCode(c, apierror.ErrReconExternalTxnsRequired, "external_transactions is required", nil)
		return
	}
	if len(req.ExternalTransactions) > model2.MaxInstantReconciliationItems {
		respondCode(c, apierror.ErrGenValidation,
			"too many external_transactions; max is "+strconv.Itoa(model2.MaxInstantReconciliationItems), nil)
		return
	}
	if len(req.MatchingRuleIDs) == 0 {
		respondCode(c, apierror.ErrReconMatchingRulesRequired, "matching_rule_ids is required", nil)
		return
	}

	reconciliationID, err := a.blnk.StartInstantReconciliation(
		c.Request.Context(),
		req.ExternalTransactions,
		req.Strategy,
		req.GroupingCriteria,
		req.MatchingRuleIDs,
		req.DryRun,
	)
	if err != nil {
		logrus.Error(err)
		respondError(c, err, withDefault(apierror.ErrReconStartFailed), withFallbackMessage("Failed to start instant reconciliation"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"reconciliation_id": reconciliationID})
}

// GetReconciliation retrieves details about a specific reconciliation by its ID.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the reconciliation ID is missing.
// - 404 Not Found: If the reconciliation cannot be found.
// - 500 Internal Server Error: If there is an error retrieving the reconciliation.
// - 200 OK: If the reconciliation is successfully retrieved.
func (a Api) GetReconciliation(c *gin.Context) {
	reconciliationID := c.Param("id")
	if reconciliationID == "" {
		respondCode(c, apierror.ErrGenMissingParameter, "Reconciliation ID is required", nil)
		return
	}

	reconciliation, err := a.blnk.GetReconciliation(c.Request.Context(), reconciliationID)
	if err != nil {
		logrus.Error(err)
		if code, ok := classifyMessage(err.Error()); ok && apierror.StatusForCode(code) == http.StatusNotFound {
			// Historical fixed message preserved for the legacy field.
			respondCode(c, apierror.ErrReconNotFound, "Reconciliation not found", nil)
			return
		}
		respondCode(c, apierror.ErrGenInternal, "Failed to retrieve reconciliation", nil)
		return
	}

	c.JSON(http.StatusOK, reconciliation)
}

// CreateMatchingRule creates a new matching rule based on the provided rule details.
// It returns the created matching rule.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the request body is invalid.
// - 500 Internal Server Error: If there is an error creating the matching rule.
// - 201 Created: If the matching rule is successfully created.
func (a Api) CreateMatchingRule(c *gin.Context) {
	var rule model.MatchingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		respondCode(c, apierror.ErrGenMalformedRequest, err.Error(), nil)
		return
	}

	createdRule, err := a.blnk.CreateMatchingRule(c.Request.Context(), rule)
	if err != nil {
		logrus.Error(err)
		respondError(c, err, withDefault(apierror.ErrGenInternal), withFallbackMessage("Failed to create matching rule"))
		return
	}

	c.JSON(http.StatusCreated, createdRule)
}

// UpdateMatchingRule updates an existing matching rule identified by its ID.
// It returns the updated matching rule.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the Matching Rule ID is missing or the request body is invalid.
// - 500 Internal Server Error: If there is an error updating the matching rule.
// - 200 OK: If the matching rule is successfully updated.
func (a Api) UpdateMatchingRule(c *gin.Context) {
	ruleID := c.Param("id")
	if ruleID == "" {
		respondCode(c, apierror.ErrGenMissingParameter, "Matching Rule ID is required", nil)
		return
	}

	var rule model.MatchingRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		respondCode(c, apierror.ErrGenMalformedRequest, err.Error(), nil)
		return
	}

	rule.RuleID = ruleID
	updatedRule, err := a.blnk.UpdateMatchingRule(c.Request.Context(), rule)
	if err != nil {
		logrus.Error(err)
		respondError(c, err, withUpgrade(apierror.ErrGenNotFound, apierror.ErrReconRuleNotFound), withDefault(apierror.ErrGenInternal), withFallbackMessage("Failed to update matching rule"))
		return
	}

	c.JSON(http.StatusOK, updatedRule)
}

// DeleteMatchingRule deletes a matching rule identified by its ID.
// It confirms the deletion with a success message.
//
// Parameters:
// - c: The Gin context containing the request and response.
//
// Responses:
// - 400 Bad Request: If the Matching Rule ID is missing.
// - 500 Internal Server Error: If there is an error deleting the matching rule.
// - 200 OK: If the matching rule is successfully deleted.
func (a Api) DeleteMatchingRule(c *gin.Context) {
	ruleID := c.Param("id")
	if ruleID == "" {
		respondCode(c, apierror.ErrGenMissingParameter, "Matching Rule ID is required", nil)
		return
	}

	err := a.blnk.DeleteMatchingRule(c.Request.Context(), ruleID)
	if err != nil {
		logrus.Error(err)
		respondError(c, err, withUpgrade(apierror.ErrGenNotFound, apierror.ErrReconRuleNotFound), withDefault(apierror.ErrGenInternal), withFallbackMessage("Failed to delete matching rule"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Matching rule deleted successfully"})
}
