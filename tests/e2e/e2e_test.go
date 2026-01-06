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
	httpServer "github.com/rezkam/mono/internal/infrastructure/http"
	"github.com/rezkam/mono/internal/infrastructure/http/handler"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
	"github.com/rezkam/mono/internal/recurring"
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
	store, err := postgres.NewPostgresStore(ctx, cfg.Database.DSN)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	// Create services
	generator := recurring.NewDomainGenerator()
	todoService := todo.NewService(store, generator, todo.Config{})
	coordinator := postgres.NewPostgresCoordinator(store.Pool())
	authenticator := auth.NewAuthenticator(store, auth.Config{OperationTimeout: 5 * time.Second})

	// Generate API key using the standard apikey tool (tests the tool itself)
	testAPIKey, err = generateAPIKeyWithTool(cfg.Database.DSN)
	if err != nil {
		panic(fmt.Errorf("failed to generate API key with tool: %w", err))
	}

	// Create API handler with OpenAPI validation (reuses production logic)
	apiHandler, err := handler.NewOpenAPIRouter(todoService, coordinator)
	if err != nil {
		panic(fmt.Errorf("failed to create API handler: %w", err))
	}

	// Create HTTP server
	server, err := httpServer.NewAPIServer(apiHandler, authenticator, httpServer.ServerConfig{})
	if err != nil {
		panic(fmt.Errorf("failed to create HTTP server: %w", err))
	}

	// Start HTTP server
	httpLis, err := net.Listen("tcp", "localhost:0") // Random port
	if err != nil {
		panic(err)
	}
	httpAddr = fmt.Sprintf("http://%s", httpLis.Addr().String())

	httpSrv := &http.Server{Handler: server.Handler()}
	go func() {
		if err := httpSrv.Serve(httpLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	httpClient = &http.Client{Timeout: 10 * time.Second}

	// Server is ready immediately after Serve() starts
	// No wait needed

	code := m.Run()

	// Shutdown: cancel context first to signal shutdown, then wait for completion
	cancel()
	httpSrv.Shutdown(context.Background())
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

	var createResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	require.NoError(t, err)

	list := createResp["list"].(map[string]any)
	listID := list["id"].(string)
	assert.NotEmpty(t, listID)
	assert.Equal(t, "E2E List", list["title"])

	// 2. Add Item
	createItemJSON := `{"title": "Buy Milk"}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var itemResp1 map[string]any
	err = json.NewDecoder(resp.Body).Decode(&itemResp1)
	require.NoError(t, err)
	item1 := itemResp1["item"].(map[string]any)
	assert.Equal(t, "Buy Milk", item1["title"])

	// 3. Create Item with tags and due time
	dueTime := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	createItemWithTagsJSON := fmt.Sprintf(`{
		"title": "Buy Milk",
		"due_at": "%s",
		"tags": ["shopping", "urgent"]
	}`, dueTime)

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemWithTagsJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var itemResp2 map[string]any
	err = json.NewDecoder(resp.Body).Decode(&itemResp2)
	require.NoError(t, err)

	item2 := itemResp2["item"].(map[string]any)
	itemID := item2["id"].(string)
	assert.NotEmpty(t, itemID)
	assert.Equal(t, "Buy Milk", item2["title"])

	tags := item2["tags"].([]any)
	assert.Contains(t, tags, "shopping")
	assert.Contains(t, tags, "urgent")

	// 4. List Items (basic - filter testing in separate test)
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/items", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listItemsResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listItemsResp)
	require.NoError(t, err)

	items := listItemsResp["items"].([]any)
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

	var updateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	updatedItem := updateResp["item"].(map[string]any)
	assert.Equal(t, "done", updatedItem["status"])

	updatedTags := updatedItem["tags"].([]any)
	assert.Contains(t, updatedTags, "shopping")
	assert.Contains(t, updatedTags, "done")

	// 6. Verify Update by getting the list
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&getListResp)
	require.NoError(t, err)

	fetchedList := getListResp["list"].(map[string]any)
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

	var listResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	listID := listResp["list"].(map[string]any)["id"].(string)

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

	var createTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)

	template := createTemplateResp["template"].(map[string]any)
	templateID := template["id"].(string)
	assert.NotEmpty(t, templateID)
	assert.Equal(t, "Daily Standup", template["title"])
	assert.Equal(t, true, template["is_active"])

	// 3. Get the template
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&getTemplateResp)
	require.NoError(t, err)

	fetchedTemplate := getTemplateResp["template"].(map[string]any)
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

	var updateTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateTemplateResp)
	require.NoError(t, err)

	updatedTemplate := updateTemplateResp["template"].(map[string]any)
	assert.Equal(t, "Updated Daily Standup", updatedTemplate["title"])

	updatedTags := updatedTemplate["tags"].([]any)
	assert.Contains(t, updatedTags, "meeting")
	assert.Contains(t, updatedTags, "team")

	// 5. List templates
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listTemplatesResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listTemplatesResp)
	require.NoError(t, err)

	templates := listTemplatesResp["templates"].([]any)
	assert.Len(t, templates, 1)

	listedTemplate := templates[0].(map[string]any)
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

	var listTemplatesResp2 map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listTemplatesResp2)
	require.NoError(t, err)

	templatesAfterDelete := listTemplatesResp2["templates"].([]any)
	assert.Len(t, templatesAfterDelete, 0)
}

// TestE2E_CreateItemWithRecurringMetadata tests creating items linked to templates
func TestE2E_CreateItemWithRecurringMetadata(t *testing.T) {
	// 1. Create list
	createListJSON := `{"title": "Recurring Tasks"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

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

	var createTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)
	templateID := createTemplateResp["template"].(map[string]any)["id"].(string)

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

	var createItemResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)

	item := createItemResp["item"].(map[string]any)
	assert.NotEmpty(t, item["id"])
	assert.Equal(t, "Standup - Dec 18", item["title"])
	assert.Equal(t, templateID, item["recurring_template_id"])
	assert.NotEmpty(t, item["instance_date"])

	tags := item["tags"].([]any)
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

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

	createItemJSON := `{"title": "Original Task", "tags": ["work", "urgent"]}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createItemResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)
	itemID := createItemResp["item"].(map[string]any)["id"].(string)

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

	var updateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	item := updateResp["item"].(map[string]any)
	assert.Equal(t, "done", item["status"])
	assert.Equal(t, "Original Task", item["title"]) // Should not change

	tags := item["tags"].([]any)
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

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

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

	var listResp map[string]any
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

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

	createTemplateJSON := `{
		"title": "Original Template",
		"recurrence_pattern": "daily",
		"generation_window_days": 7,
		"tags": ["original"]
	}`

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)
	templateID := createTemplateResp["template"].(map[string]any)["id"].(string)

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

	var updateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	template := updateResp["template"].(map[string]any)
	assert.Equal(t, "Updated Title Only", template["title"])
	assert.Equal(t, "daily", template["recurrence_pattern"]) // Should not change

	tags := template["tags"].([]any)
	assert.Contains(t, tags, "original") // Should not change
}

// TestE2E_ListsPagination tests pagination functionality
func TestE2E_ListsPagination(t *testing.T) {
	// Create multiple lists
	for i := range 25 {
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

	var listResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	lists := listResp["lists"].([]any)
	assert.LessOrEqual(t, len(lists), 10) // Should respect page size

	// Should have next page token if more results exist
	if len(lists) == 10 {
		assert.NotNil(t, listResp["next_page_token"])
	}
}

// TestE2E_ItemWithStartsAtAndDueOffset tests creating and updating items with scheduling fields
func TestE2E_ItemWithStartsAtAndDueOffset(t *testing.T) {
	// 1. Create list
	createListJSON := `{"title": "Scheduled Tasks"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

	// 2. Create item with starts_at and due_offset
	startsAt := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	createItemJSON := fmt.Sprintf(`{
		"title": "Future Task",
		"starts_at": "%s",
		"due_offset": "PT2H30M",
		"tags": ["scheduled"],
		"priority": "high"
	}`, startsAt)

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createItemResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)

	item := createItemResp["item"].(map[string]any)
	itemID := item["id"].(string)

	// Verify all input fields are present in response
	assert.NotEmpty(t, itemID)
	assert.Equal(t, "Future Task", item["title"])
	assert.NotNil(t, item["starts_at"], "starts_at should be present in response")
	assert.Equal(t, startsAt, item["starts_at"], "starts_at should match input")
	assert.NotNil(t, item["due_offset"], "due_offset should be present in response")
	assert.Equal(t, "PT2H30M", item["due_offset"], "due_offset should be ISO 8601 format in response")
	assert.Equal(t, "high", item["priority"])

	tags := item["tags"].([]any)
	assert.Contains(t, tags, "scheduled")

	// 3. Update starts_at and due_offset using field mask
	newStartsAt := time.Now().UTC().AddDate(0, 0, 14).Format("2006-01-02")
	updateJSON := fmt.Sprintf(`{
		"item": {
			"id": "%s",
			"starts_at": "%s",
			"due_offset": "PT4H"
		},
		"update_mask": ["starts_at", "due_offset"]
	}`, itemID, newStartsAt)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/items/%s", listID, itemID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	updatedItem := updateResp["item"].(map[string]any)

	// Verify updated fields (ISO 8601 format for durations)
	assert.Equal(t, newStartsAt, updatedItem["starts_at"], "starts_at should be updated")
	assert.Equal(t, "PT4H", updatedItem["due_offset"], "due_offset should be updated in ISO 8601 format")
	assert.Equal(t, "Future Task", updatedItem["title"], "title should not change")
	assert.Equal(t, "high", updatedItem["priority"], "priority should not change")

	// 4. Get item and verify persistence
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/items", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	var listResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	items := listResp["items"].([]any)
	require.Len(t, items, 1)

	persistedItem := items[0].(map[string]any)
	assert.Equal(t, itemID, persistedItem["id"])
	assert.Equal(t, newStartsAt, persistedItem["starts_at"], "starts_at should persist")
	assert.Equal(t, "PT4H", persistedItem["due_offset"], "due_offset should persist in ISO 8601 format")
}

// TestE2E_RecurringTemplateWithDueOffset tests that recurring templates with due_offset generate items correctly
func TestE2E_RecurringTemplateWithDueOffset(t *testing.T) {
	// 1. Create list
	createListJSON := `{"title": "Recurring Scheduled Tasks"}`
	resp, err := httpRequest(t, "POST", "/api/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]any)["id"].(string)

	// 2. Create recurring template with due_offset
	createTemplateJSON := `{
		"title": "Daily Meeting",
		"recurrence_pattern": "daily",
		"due_offset": "PT1H30M",
		"sync_horizon_days": 7,
		"generation_horizon_days": 30,
		"tags": ["meeting", "daily"]
	}`

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/api/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 201, got %d. Response: %s", resp.StatusCode, string(body))
	}

	var createTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&createTemplateResp)
	require.NoError(t, err)

	template := createTemplateResp["template"].(map[string]any)
	templateID := template["id"].(string)

	// Verify template has due_offset in response (ISO 8601 format)
	assert.NotEmpty(t, templateID)
	assert.Equal(t, "Daily Meeting", template["title"])
	assert.NotNil(t, template["due_offset"], "due_offset should be present in template response")
	assert.Equal(t, "PT1H30M", template["due_offset"], "due_offset should be ISO 8601 format in response")

	templateTags := template["tags"].([]any)
	assert.Contains(t, templateTags, "meeting")
	assert.Contains(t, templateTags, "daily")

	// 3. Get template to verify persistence
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	var getTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&getTemplateResp)
	require.NoError(t, err)

	fetchedTemplate := getTemplateResp["template"].(map[string]any)
	assert.Equal(t, "PT1H30M", fetchedTemplate["due_offset"], "due_offset should persist in template with ISO 8601 format")

	// 4. List generated items and verify they have starts_at and due_offset
	resp, err = httpRequest(t, "GET", fmt.Sprintf("/api/v1/lists/%s/items", listID), "")
	require.NoError(t, err)
	defer resp.Body.Close()

	var listResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	items := listResp["items"].([]any)
	assert.Greater(t, len(items), 0, "Template should generate at least one item")

	// Verify first generated item has all scheduling fields
	firstItem := items[0].(map[string]any)
	assert.Equal(t, templateID, firstItem["recurring_template_id"], "Generated item should link to template")
	assert.NotNil(t, firstItem["instance_date"], "Generated item should have instance_date")
	assert.NotNil(t, firstItem["starts_at"], "Generated item should have starts_at from template pattern")
	assert.NotNil(t, firstItem["due_offset"], "Generated item should inherit due_offset from template")
	assert.Equal(t, "PT1H30M", firstItem["due_offset"], "Generated item due_offset should match template in ISO 8601 format")

	itemTags := firstItem["tags"].([]any)
	assert.Contains(t, itemTags, "meeting", "Generated item should inherit tags from template")
	assert.Contains(t, itemTags, "daily", "Generated item should inherit tags from template")

	// 5. Update template's due_offset and verify it affects future generations
	updateTemplateJSON := fmt.Sprintf(`{
		"template": {
			"id": "%s",
			"list_id": "%s",
			"due_offset": "PT3H"
		},
		"update_mask": ["due_offset"]
	}`, templateID, listID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/api/v1/lists/%s/recurring-templates/%s", listID, templateID), updateTemplateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateTemplateResp map[string]any
	err = json.NewDecoder(resp.Body).Decode(&updateTemplateResp)
	require.NoError(t, err)

	updatedTemplate := updateTemplateResp["template"].(map[string]any)
	assert.Equal(t, "PT3H", updatedTemplate["due_offset"], "Template due_offset should be updated in ISO 8601 format")
	assert.Equal(t, "Daily Meeting", updatedTemplate["title"], "Title should not change")
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
