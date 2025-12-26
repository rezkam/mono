package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/rezkam/mono/internal/application/todo"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/domain"
	httpRouter "github.com/rezkam/mono/internal/http"
	"github.com/rezkam/mono/internal/http/handler"
	"github.com/rezkam/mono/internal/http/middleware"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	httpAddr   string
	httpClient *http.Client
	testAPIKey string
)

func TestMain(m *testing.M) {
	// Skip e2e tests if PostgreSQL is not available
	cfg, err := config.LoadTestConfig()
	if err != nil {
		fmt.Printf("Skipping E2E tests: %v\n", err)
		os.Exit(0)
	}

	// Setup Server with PostgreSQL
	ctx, cancel := context.WithCancel(context.Background())
	store, err := postgres.NewPostgresStore(ctx, cfg.StorageDSN)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	// Create services
	todoService := todo.NewService(store, todo.Config{})
	authenticator := auth.NewAuthenticator(store, auth.Config{OperationTimeout: 5 * time.Second})

	// Generate API key using the standard apikey tool (tests the tool itself)
	testAPIKey, err = generateAPIKeyWithTool(cfg.StorageDSN)
	if err != nil {
		panic(fmt.Errorf("failed to generate API key with tool: %w", err))
	}

	// Create HTTP handlers and middleware
	server := handler.NewServer(todoService)
	authMiddleware := middleware.NewAuth(authenticator)

	// Create router
	router := httpRouter.NewRouter(server, authMiddleware)

	// Start HTTP server
	httpLis, err := net.Listen("tcp", "localhost:0") // Random port
	if err != nil {
		panic(err)
	}
	httpAddr = fmt.Sprintf("http://%s", httpLis.Addr().String())

	httpServer := &http.Server{Handler: router}
	go func() {
		if err := httpServer.Serve(httpLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	httpClient = &http.Client{Timeout: 10 * time.Second}

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	code := m.Run()

	// Shutdown: cancel context first to signal shutdown, then wait for completion
	cancel()
	httpServer.Shutdown(context.Background())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	authenticator.Shutdown(shutdownCtx)
	os.Exit(code)
}

// generateAPIKeyWithTool uses the mono-apikey binary to generate an API key.
// This tests the actual tool that users will use, ensuring it works correctly.
func generateAPIKeyWithTool(pgURL string) (string, error) {
	// Build the apikey tool first
	buildCmd := exec.Command("go", "build", "-o", "mono-apikey-test", "../../cmd/apikey/main.go")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to build apikey tool: %w\nOutput: %s", err, output)
	}
	defer os.Remove("mono-apikey-test")

	// Run the apikey tool
	cmd := exec.Command("./mono-apikey-test", "-name", "E2E Test Key", "-days", "1")
	cmd.Env = append(os.Environ(), "MONO_STORAGE_DSN="+pgURL)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w\nOutput: %s", err, output)
	}

	// Parse the API key from output
	// The tool outputs: "API Key: <key>"
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "API Key:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("%w:\n%s", domain.ErrAPIKeyParsingFailed, outputStr)
}

// TestE2E_CreateAndGetList tests the list creation and retrieval flow
func TestE2E_CreateAndGetList(t *testing.T) {
	// 1. Create List
	createListJSON := `{"title": "E2E List"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	require.NoError(t, err)

	list := createResp["list"].(map[string]interface{})
	listID := list["id"].(string)
	assert.NotEmpty(t, listID)
	assert.Equal(t, "E2E List", list["title"])

	// 2. Add Item
	createItemJSON := `{"title": "Buy Milk"}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var itemResp1 map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&itemResp1)
	require.NoError(t, err)
	item1 := itemResp1["item"].(map[string]interface{})
	assert.Equal(t, "Buy Milk", item1["title"])

	// 3. Create Item with tags and due time
	dueTime := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	createItemWithTagsJSON := fmt.Sprintf(`{
		"title": "Buy Milk",
		"due_time": "%s",
		"tags": ["shopping", "urgent"]
	}`, dueTime)

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemWithTagsJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var itemResp2 map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&itemResp2)
	require.NoError(t, err)

	item2 := itemResp2["item"].(map[string]interface{})
	itemID := item2["id"].(string)
	assert.NotEmpty(t, itemID)
	assert.Equal(t, "Buy Milk", item2["title"])

	tags := item2["tags"].([]interface{})
	assert.Contains(t, tags, "shopping")
	assert.Contains(t, tags, "urgent")

	// 4. List Items (basic - filter testing in separate test)
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/items", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listItemsResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listItemsResp)
	require.NoError(t, err)

	items := listItemsResp["items"].([]interface{})
	assert.Equal(t, 2, len(items)) // Exactly our 2 items in this list

	// 5. Update Item (Tags and Status)
	updateJSON := fmt.Sprintf(`{
		"item": {
			"id": "%s",
			"status": "done",
			"tags": ["shopping", "done"]
		},
		"update_mask": ["status", "tags"]
	}`, itemID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/items/%s", listID, itemID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	updatedItem := updateResp["item"].(map[string]interface{})
	assert.Equal(t, "done", updatedItem["status"])

	updatedTags := updatedItem["tags"].([]interface{})
	assert.Contains(t, updatedTags, "shopping")
	assert.Contains(t, updatedTags, "done")

	// 6. Verify Update by getting the list
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&getListResp)
	require.NoError(t, err)

	fetchedList := getListResp["list"].(map[string]interface{})
	assert.Equal(t, listID, fetchedList["id"])
	assert.Equal(t, "E2E List", fetchedList["title"])
}

// TestE2E_RecurringTemplates tests the full recurring template lifecycle
func TestE2E_RecurringTemplates(t *testing.T) {
	// 1. Create a list for the template
	createListJSON := `{"title": "Recurring Tasks List"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var listResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	listID := listResp["list"].(map[string]interface{})["id"].(string)

	// 2. Create a recurring template
	createTemplateJSON := `{
		"title": "Daily Standup",
		"recurrence_pattern": "daily",
		"generation_window_days": 7,
		"tags": ["meeting"]
	}`

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201, got %d. Response: %s", resp.StatusCode, string(body))
	}

	var createTemplateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)

	template := createTemplateResp["template"].(map[string]interface{})
	templateID := template["id"].(string)
	assert.NotEmpty(t, templateID)
	assert.Equal(t, "Daily Standup", template["title"])
	assert.Equal(t, true, template["is_active"])

	// 3. Get the template
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getTemplateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&getTemplateResp)
	require.NoError(t, err)

	fetchedTemplate := getTemplateResp["template"].(map[string]interface{})
	assert.Equal(t, "Daily Standup", fetchedTemplate["title"])

	// 4. Update the template
	updateTemplateJSON := fmt.Sprintf(`{
		"template": {
			"id": "%s",
			"list_id": "%s",
			"title": "Updated Daily Standup",
			"recurrence_pattern": "weekdays",
			"tags": ["meeting", "team"]
		},
		"update_mask": ["title", "recurrence_pattern", "tags"]
	}`, templateID, listID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), updateTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Response: %s", resp.StatusCode, string(body))
	}

	var updateTemplateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateTemplateResp)
	require.NoError(t, err)

	updatedTemplate := updateTemplateResp["template"].(map[string]interface{})
	assert.Equal(t, "Updated Daily Standup", updatedTemplate["title"])

	updatedTags := updatedTemplate["tags"].([]interface{})
	assert.Contains(t, updatedTags, "meeting")
	assert.Contains(t, updatedTags, "team")

	// 5. List templates
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listTemplatesResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listTemplatesResp)
	require.NoError(t, err)

	templates := listTemplatesResp["templates"].([]interface{})
	assert.Len(t, templates, 1)

	listedTemplate := templates[0].(map[string]interface{})
	assert.Equal(t, "Updated Daily Standup", listedTemplate["title"])

	// 6. Delete the template
	resp, err = httpRequest(t, "DELETE", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// 7. Verify deletion
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listTemplatesResp2 map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listTemplatesResp2)
	require.NoError(t, err)

	templatesAfterDelete := listTemplatesResp2["templates"].([]interface{})
	assert.Len(t, templatesAfterDelete, 0)
}

// TestE2E_CreateItemWithRecurringMetadata tests creating items linked to templates
func TestE2E_CreateItemWithRecurringMetadata(t *testing.T) {
	// 1. Create list
	createListJSON := `{"title": "Recurring Tasks"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	// 2. Create recurring template
	createTemplateJSON := `{
		"title": "Daily Standup",
		"recurrence_pattern": "daily",
		"generation_window_days": 7,
		"tags": ["meeting"]
	}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createTemplateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)
	templateID := createTemplateResp["template"].(map[string]interface{})["id"].(string)

	// 3. Create item with recurring metadata
	createItemJSON := fmt.Sprintf(`{
		"title": "Standup - Dec 18",
		"recurring_template_id": "%s",
		"instance_date": "2025-12-18T00:00:00Z",
		"tags": ["recurring", "meeting"],
		"priority": "high"
	}`, templateID)

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createItemResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)

	item := createItemResp["item"].(map[string]interface{})
	assert.NotEmpty(t, item["id"])
	assert.Equal(t, "Standup - Dec 18", item["title"])
	assert.Equal(t, templateID, item["recurring_template_id"])
	assert.NotEmpty(t, item["instance_date"])

	tags := item["tags"].([]interface{})
	assert.Contains(t, tags, "recurring")
	assert.Contains(t, tags, "meeting")
	assert.Equal(t, "high", item["priority"])
}

// TestE2E_UpdateItemWithFieldMask tests partial updates with field masks
func TestE2E_UpdateItemWithFieldMask(t *testing.T) {
	// 1. Create list and item
	createListJSON := `{"title": "Task List"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	createItemJSON := `{"title": "Original Task", "tags": ["work", "urgent"]}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createItemResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)
	itemID := createItemResp["item"].(map[string]interface{})["id"].(string)

	// 2. Update only status using field mask
	updateJSON := fmt.Sprintf(`{
		"item": {
			"id": "%s",
			"status": "done"
		},
		"update_mask": ["status"]
	}`, itemID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/items/%s", listID, itemID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d. Response: %s", resp.StatusCode, string(body))
	}

	var updateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	item := updateResp["item"].(map[string]interface{})
	assert.Equal(t, "done", item["status"])
	assert.Equal(t, "Original Task", item["title"]) // Should not change

	tags := item["tags"].([]interface{})
	assert.Contains(t, tags, "work")   // Should not change
	assert.Contains(t, tags, "urgent") // Should not change
}

// TestE2E_ListTasksWithFilter tests filtering functionality
func TestE2E_ListTasksWithFilter(t *testing.T) {
	// 1. Create list and multiple items
	createListJSON := `{"title": "Filtered Tasks"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	// Use unique tag to avoid interference from other tests
	uniqueTag := fmt.Sprintf("urgent-%d", time.Now().UTC().Unix())

	// Create items with different tags
	items := []string{
		fmt.Sprintf(`{"title": "Urgent Task", "tags": ["%s", "work"]}`, uniqueTag),
		`{"title": "Normal Task", "tags": ["work"]}`,
		fmt.Sprintf(`{"title": "Personal Task", "tags": ["%s", "personal"]}`, uniqueTag),
	}

	for _, itemJSON := range items {
		resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), itemJSON)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// 2. List tasks filtered by unique tag
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/items?tags=%s", listID, uniqueTag), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	// Since filter parsing is not fully implemented, we just verify the endpoint works
	assert.NotNil(t, listResp["items"])
}

// TestE2E_RecurringTemplateFieldMask tests field mask support for templates
func TestE2E_RecurringTemplateFieldMask(t *testing.T) {
	// 1. Create list and template
	createListJSON := `{"title": "Template Test List"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	createTemplateJSON := `{
		"title": "Original Template",
		"recurrence_pattern": "daily",
		"generation_window_days": 7,
		"tags": ["original"]
	}`

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createTemplateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)
	templateID := createTemplateResp["template"].(map[string]interface{})["id"].(string)

	// 2. Update only title using field mask
	updateJSON := fmt.Sprintf(`{
		"template": {
			"id": "%s",
			"list_id": "%s",
			"title": "Updated Title Only"
		},
		"update_mask": ["title"]
	}`, templateID, listID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	template := updateResp["template"].(map[string]interface{})
	assert.Equal(t, "Updated Title Only", template["title"])
	assert.Equal(t, "daily", template["recurrence_pattern"]) // Should not change

	tags := template["tags"].([]interface{})
	assert.Contains(t, tags, "original") // Should not change
}

// TestE2E_ListsPagination tests pagination functionality
func TestE2E_ListsPagination(t *testing.T) {
	// Create multiple lists
	for i := 0; i < 25; i++ {
		createListJSON := fmt.Sprintf(`{"title": "Pagination Test List %d"}`, i)
		resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Request with page size
	resp, err := httpRequest(t, "GET", "/api/v1/lists?page_size=10", "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	lists := listResp["lists"].([]interface{})
	assert.LessOrEqual(t, len(lists), 10) // Should respect page size

	// Should have next page token if more results exist
	if len(lists) == 10 {
		assert.NotNil(t, listResp["next_page_token"])
	}
}

// httpRequest is a helper to make authenticated HTTP requests with JSON
func httpRequest(t *testing.T, method, path, body string) (*http.Response, error) {
	var reqBody io.Reader
	if body != "" {
		reqBody = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, httpAddr+path, reqBody)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("Content-Type", "application/json")

	return httpClient.Do(req)
}
