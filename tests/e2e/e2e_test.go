package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/auth"
	"github.com/rezkam/mono/internal/service"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	serverAddr string
	httpAddr   string
	httpClient *http.Client
	testAPIKey string
)

func TestMain(m *testing.M) {
	// Skip e2e tests if PostgreSQL is not available
	pgURL := os.Getenv("TEST_POSTGRES_URL")
	if pgURL == "" {
		// Log that we are skipping
		println("Skipping E2E tests: TEST_POSTGRES_URL is not set")
		os.Exit(0)
	}

	// Setup Server with PostgreSQL
	ctx := context.Background()
	store, err := sqlstorage.NewPostgresStore(ctx, pgURL)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	svc := service.NewMonoService(store, 50, 100)

	// Generate API key using the standard apikey tool (tests the tool itself)
	testAPIKey, err = generateAPIKeyWithTool(pgURL)
	if err != nil {
		panic(fmt.Errorf("failed to generate API key with tool: %w", err))
	}

	// Start gRPC server with auth interceptor
	authenticator := auth.NewAuthenticator(store.DB(), store.Queries())
	lis, err := net.Listen("tcp", "localhost:0") // Random port
	if err != nil {
		panic(err)
	}
	serverAddr = lis.Addr().String()

	s := grpc.NewServer(
		grpc.UnaryInterceptor(authenticator.UnaryInterceptor),
	)
	monov1.RegisterMonoServiceServer(s, svc)

	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()

	// Start HTTP/REST Gateway
	httpLis, err := net.Listen("tcp", "localhost:0") // Random port
	if err != nil {
		panic(err)
	}
	httpAddr = fmt.Sprintf("http://%s", httpLis.Addr().String())

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	err = monov1.RegisterMonoServiceHandlerFromEndpoint(ctx, mux, serverAddr, opts)
	if err != nil {
		panic(err)
	}

	httpServer := &http.Server{Handler: mux}
	go func() {
		if err := httpServer.Serve(httpLis); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	httpClient = &http.Client{Timeout: 10 * time.Second}

	// Give servers a moment to start
	time.Sleep(100 * time.Millisecond)

	code := m.Run()

	s.Stop()
	httpServer.Shutdown(ctx)
	os.Exit(code)
}

// authContext returns a context with the API key attached
func authContext() context.Context {
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + testAPIKey,
	})
	return metadata.NewOutgoingContext(context.Background(), md)
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
	cmd.Env = append(os.Environ(), "POSTGRES_URL="+pgURL)

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

	return "", fmt.Errorf("could not parse API key from tool output:\n%s", outputStr)
}

func TestE2E_CreateAndGetList_gRPC(t *testing.T) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := monov1.NewMonoServiceClient(conn)
	ctx := authContext()

	// 1. Create List
	createResp, err := client.CreateList(ctx, &monov1.CreateListRequest{Title: "E2E List"})
	require.NoError(t, err)
	assert.NotEmpty(t, createResp.List.Id)
	assert.Equal(t, "E2E List", createResp.List.Title)

	// 2. Add Item
	itemResp, err := client.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId: createResp.List.Id,
		Title:  "Buy Milk",
	})
	require.NoError(t, err)
	assert.Equal(t, "Buy Milk", itemResp.Item.Title)

	// 3. Create Item
	// Use explicit time for due date (e.g. 24h from now)
	dueTime := time.Now().Add(24 * time.Hour)
	itemResp, err = client.CreateItem(ctx, &monov1.CreateItemRequest{
		ListId:  createResp.List.Id,
		Title:   "Buy Milk",
		DueTime: timestamppb.New(dueTime),
		Tags:    []string{"shopping", "urgent"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, itemResp.Item.Id)
	assert.Equal(t, "Buy Milk", itemResp.Item.Title)
	assert.Equal(t, []string{"shopping", "urgent"}, itemResp.Item.Tags)

	// 4. List Tasks (Filter)
	listTasksResp, err := client.ListTasks(ctx, &monov1.ListTasksRequest{
		Filter: "tags:urgent",
	})
	require.NoError(t, err)
	assert.Len(t, listTasksResp.Items, 1)
	assert.Equal(t, itemResp.Item.Id, listTasksResp.Items[0].Id)

	// 5. Update Item (Tags and Completed)
	updateResp, err := client.UpdateItem(ctx, &monov1.UpdateItemRequest{
		ListId: createResp.List.Id,
		Item: &monov1.TodoItem{
			Id:     itemResp.Item.Id,
			Status: monov1.TaskStatus_TASK_STATUS_DONE,
			Tags:   []string{"shopping", "done"},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status", "tags"}},
	})
	require.NoError(t, err)
	assert.Equal(t, monov1.TaskStatus_TASK_STATUS_DONE, updateResp.Item.Status)
	assert.Equal(t, []string{"shopping", "done"}, updateResp.Item.Tags)

	// 6. Verify Update
	getResp2, err := client.GetList(ctx, &monov1.GetListRequest{Id: createResp.List.Id})
	require.NoError(t, err)
	assert.Equal(t, monov1.TaskStatus_TASK_STATUS_DONE, getResp2.List.Items[1].Status) // Assuming the second item is the one updated
	assert.Equal(t, []string{"shopping", "done"}, getResp2.List.Items[1].Tags)
}

func TestE2E_RecurringTemplates_gRPC(t *testing.T) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := monov1.NewMonoServiceClient(conn)
	ctx := authContext()

	// 1. Create a list for the template
	listResp, err := client.CreateList(ctx, &monov1.CreateListRequest{Title: "Recurring Tasks List"})
	require.NoError(t, err)

	// 2. Create a recurring template
	createTemplateResp, err := client.CreateRecurringTemplate(ctx, &monov1.CreateRecurringTemplateRequest{
		ListId:               listResp.List.Id,
		Title:                "Daily Standup",
		RecurrencePattern:    monov1.RecurrencePattern_RECURRENCE_PATTERN_DAILY,
		GenerationWindowDays: 7,
		Tags:                 []string{"meeting"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, createTemplateResp.Template.Id)
	assert.Equal(t, "Daily Standup", createTemplateResp.Template.Title)
	assert.True(t, createTemplateResp.Template.IsActive)

	// 3. Get the template
	getTemplateResp, err := client.GetRecurringTemplate(ctx, &monov1.GetRecurringTemplateRequest{
		Id: createTemplateResp.Template.Id,
	})
	require.NoError(t, err)
	assert.Equal(t, "Daily Standup", getTemplateResp.Template.Title)

	// 4. Update the template
	updateTemplateResp, err := client.UpdateRecurringTemplate(ctx, &monov1.UpdateRecurringTemplateRequest{
		Template: &monov1.RecurringTaskTemplate{
			Id:                createTemplateResp.Template.Id,
			ListId:            listResp.List.Id,
			Title:             "Updated Daily Standup",
			RecurrencePattern: monov1.RecurrencePattern_RECURRENCE_PATTERN_WEEKDAYS,
			Tags:              []string{"meeting", "team"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Daily Standup", updateTemplateResp.Template.Title)
	assert.Equal(t, []string{"meeting", "team"}, updateTemplateResp.Template.Tags)

	// 5. List templates
	listTemplatesResp, err := client.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
		ListId: listResp.List.Id,
	})
	require.NoError(t, err)
	assert.Len(t, listTemplatesResp.Templates, 1)
	assert.Equal(t, "Updated Daily Standup", listTemplatesResp.Templates[0].Title)

	// 6. Delete the template
	_, err = client.DeleteRecurringTemplate(ctx, &monov1.DeleteRecurringTemplateRequest{
		Id: createTemplateResp.Template.Id,
	})
	require.NoError(t, err)

	// 7. Verify deletion
	listTemplatesResp, err = client.ListRecurringTemplates(ctx, &monov1.ListRecurringTemplatesRequest{
		ListId: listResp.List.Id,
	})
	require.NoError(t, err)
	assert.Len(t, listTemplatesResp.Templates, 0)
}

// HTTP/REST API Tests with JSON payloads

func TestE2E_CreateAndGetList_HTTP(t *testing.T) {
	// 1. Create List via HTTP POST with JSON
	createListJSON := `{"title": "Shopping List"}`
	resp, err := httpRequest(t, "POST", "/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var createResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	require.NoError(t, err)

	list := createResp["list"].(map[string]interface{})
	listID := list["id"].(string)
	assert.NotEmpty(t, listID)
	assert.Equal(t, "Shopping List", list["title"])

	// 2. Get List via HTTP GET
	resp, err = httpRequest(t, "GET", "/v1/lists/"+listID, "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&getResp)
	require.NoError(t, err)

	list = getResp["list"].(map[string]interface{})
	assert.Equal(t, listID, list["id"])
	assert.Equal(t, "Shopping List", list["title"])
}

func TestE2E_CreateItemWithRecurringMetadata_HTTP(t *testing.T) {
	// 1. Create list
	createListJSON := `{"title": "Recurring Tasks"}`
	resp, err := httpRequest(t, "POST", "/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	// 2. Create recurring template
	createTemplateJSON := `{
		"title": "Daily Standup",
		"recurrence_pattern": "RECURRENCE_PATTERN_DAILY",
		"generation_window_days": 7,
		"tags": ["meeting"]
	}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
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
		"priority": "TASK_PRIORITY_HIGH"
	}`, templateID)

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/v1/lists/%s/items", listID), createItemJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var createItemResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createItemResp)
	require.NoError(t, err)

	item := createItemResp["item"].(map[string]interface{})
	assert.NotEmpty(t, item["id"])
	assert.Equal(t, "Standup - Dec 18", item["title"])
	assert.Equal(t, templateID, item["recurringTemplateId"])
	assert.NotEmpty(t, item["instanceDate"])
	assert.Contains(t, item["tags"], "recurring")
	assert.Contains(t, item["tags"], "meeting")
	assert.Equal(t, "TASK_PRIORITY_HIGH", item["priority"])
}

func TestE2E_UpdateItemWithFieldMask_HTTP(t *testing.T) {
	// 1. Create list and item
	createListJSON := `{"title": "Task List"}`
	resp, err := httpRequest(t, "POST", "/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	createItemJSON := `{"title": "Original Task", "tags": ["work", "urgent"]}`
	resp, err = httpRequest(t, "POST", fmt.Sprintf("/v1/lists/%s/items", listID), createItemJSON)
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
			"status": "TASK_STATUS_DONE"
		},
		"update_mask": "status"
	}`, itemID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/v1/lists/%s/items/%s", listID, itemID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	item := updateResp["item"].(map[string]interface{})
	assert.Equal(t, "TASK_STATUS_DONE", item["status"])
	assert.Equal(t, "Original Task", item["title"]) // Should not change
	assert.Contains(t, item["tags"], "work")        // Should not change
	assert.Contains(t, item["tags"], "urgent")      // Should not change
}

func TestE2E_ListTasksWithFilter_HTTP(t *testing.T) {
	// 1. Create list and multiple items
	createListJSON := `{"title": "Filtered Tasks"}`
	resp, err := httpRequest(t, "POST", "/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	// Create items with different tags
	items := []string{
		`{"title": "Urgent Task", "tags": ["urgent", "work"]}`,
		`{"title": "Normal Task", "tags": ["work"]}`,
		`{"title": "Personal Task", "tags": ["urgent", "personal"]}`,
	}

	for _, itemJSON := range items {
		resp, err = httpRequest(t, "POST", fmt.Sprintf("/v1/lists/%s/items", listID), itemJSON)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// 2. List tasks filtered by "urgent" tag
	resp, err = httpRequest(t, "GET", "/v1/tasks?filter=tags:urgent", "")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	require.NoError(t, err)

	itemsRaw := listResp["items"].([]interface{})
	assert.Len(t, itemsRaw, 2) // Should find 2 items with "urgent" tag

	for _, itemRaw := range itemsRaw {
		itemMap := itemRaw.(map[string]interface{})
		tags := itemMap["tags"].([]interface{})
		assert.Contains(t, tags, "urgent")
	}
}

func TestE2E_RecurringTemplateFieldMask_HTTP(t *testing.T) {
	// 1. Create list and template
	createListJSON := `{"title": "Template Test List"}`
	resp, err := httpRequest(t, "POST", "/v1/lists", createListJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	var createListResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createListResp)
	require.NoError(t, err)
	listID := createListResp["list"].(map[string]interface{})["id"].(string)

	createTemplateJSON := `{
		"title": "Original Template",
		"recurrence_pattern": "RECURRENCE_PATTERN_DAILY",
		"generation_window_days": 7,
		"tags": ["original"]
	}`

	resp, err = httpRequest(t, "POST", fmt.Sprintf("/v1/lists/%s/recurring-templates", listID), createTemplateJSON)
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
		"update_mask": "title"
	}`, templateID, listID)

	resp, err = httpRequest(t, "PATCH", fmt.Sprintf("/v1/lists/%s/recurring-templates/%s", listID, templateID), updateJSON)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&updateResp)
	require.NoError(t, err)

	template := updateResp["template"].(map[string]interface{})
	assert.Equal(t, "Updated Title Only", template["title"])
	assert.Equal(t, "RECURRENCE_PATTERN_DAILY", template["recurrencePattern"]) // Should not change
	assert.Contains(t, template["tags"], "original")                           // Should not change
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
