package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rezkam/mono/internal/domain"
	postgres "github.com/rezkam/mono/internal/infrastructure/persistence/postgres"
)

// setupSQLInjectionTest initializes test environment for SQL injection testing
func setupSQLInjectionTest(t *testing.T) (*postgres.Store, func()) {
	pgURL := GetTestStorageDSN(t)

	ctx := context.Background()
	store, err := postgres.NewPostgresStore(ctx, pgURL)
	require.NoError(t, err)

	// Clean up tables before test
	_, err = store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
	require.NoError(t, err)

	cleanup := func() {
		store.Pool().Exec(ctx, "TRUNCATE TABLE todo_items, todo_lists, task_status_history, recurring_task_templates, recurring_generation_jobs, api_keys CASCADE")
		store.Close()
	}

	return store, cleanup
}

// TestListTasks_DirectAssignment_SQLInjectionResistance tests the EXACT scenario mentioned:
//
// "In the service layer: params.OrderBy = req.OrderBy  // Direct assignment, no validation"
//
// This test proves that even with direct assignment from user input to storage params,
// SQL injection is IMPOSSIBLE because parameterized queries treat input as data, not code.
func TestListTasks_DirectAssignment_SQLInjectionResistance(t *testing.T) {
	store, cleanup := setupSQLInjectionTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	listUUID, err := uuid.NewV7()
	require.NoError(t, err, "failed to generate list UUID")
	listID := listUUID.String()
	list := &domain.TodoList{
		ID:         listID,
		Title:      "SQL Injection Test List",
		Items:      []domain.TodoItem{},
		CreateTime: time.Now(),
	}
	err = store.CreateList(ctx, list)
	require.NoError(t, err)

	// Create test tasks
	taskIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		taskUUID, err := uuid.NewV7()
		require.NoError(t, err, "failed to generate task UUID")
		taskID := taskUUID.String()
		taskIDs[i] = taskID
		priorityMedium := domain.TaskPriorityMedium
		task := domain.TodoItem{
			ID:       taskID,
			Title:    "Test Task",
			Status:   domain.TaskStatusTodo,
			Priority: &priorityMedium,
		}
		err = store.CreateItem(ctx, listID, &task)
		require.NoError(t, err)
	}

	// Test SQL injection attempts that flow directly to database WITHOUT validation
	injectionAttempts := []struct {
		name            string
		orderBy         string
		description     string
		flowExplanation string
	}{
		{
			name:        "DROP_TABLE_attack",
			orderBy:     "id; DROP TABLE todo_items--",
			description: "Attempts to drop the entire table",
			flowExplanation: `
Flow (simulating service layer direct assignment):
1. User Input: orderBy = "id; DROP TABLE todo_items--"
2. Service Layer (mono.go:363): params.OrderBy = req.OrderBy  // Direct assignment!
3. Storage Layer: ListTasks(params) receives OrderBy = "id; DROP TABLE todo_items--"
4. SQL Query (todo_items.sql:87):
   ORDER BY CASE WHEN $9::text = 'due_time' THEN due_time END ...

   PostgreSQL sees: CASE WHEN 'id; DROP TABLE todo_items--' = 'due_time'
                           ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                           STRING LITERAL, not executable SQL

5. Result: Safe execution, table intact`,
		},
		{
			name:        "DELETE_attack",
			orderBy:     "id; DELETE FROM todo_items WHERE 1=1--",
			description: "Attempts to delete all tasks",
			flowExplanation: `
Flow:
1. User Input: "id; DELETE FROM todo_items WHERE 1=1--"
2. Service: params.OrderBy = userInput  // No validation!
3. SQL: Parameter $9 = 'id; DELETE FROM todo_items WHERE 1=1--'
4. PostgreSQL: Compares string in CASE, never executes DELETE
5. Result: All tasks safe`,
		},
		{
			name:        "UPDATE_injection",
			orderBy:     "id; UPDATE todo_items SET status='DONE'--",
			description: "Attempts to modify all tasks to DONE status",
			flowExplanation: `
Flow:
1. Malicious input flows through service layer without validation
2. Storage receives: OrderBy = "id; UPDATE todo_items SET status='DONE'--"
3. SQL: $9::text parameter contains the malicious string
4. PostgreSQL: $9 is typed as text, treated as data not code
5. Result: Tasks remain TODO status`,
		},
	}

	for _, tc := range injectionAttempts {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate service layer: params.OrderBy = req.OrderBy (direct assignment)
			params := domain.ListTasksParams{
				ListID:  &listID,
				OrderBy: tc.orderBy, // MALICIOUS INPUT - No validation!
				Limit:   10,
				Offset:  0,
			}

			// Execute query with malicious input
			result, err := store.FindItems(ctx, params)

			// CRITICAL ASSERTIONS PROVING SAFETY

			// 1. Query executes successfully despite malicious input
			require.NoError(t, err,
				"Query should execute safely even with SQL injection attempt.\n%s",
				tc.flowExplanation)

			// 2. All tasks returned (none deleted)
			assert.GreaterOrEqual(t, len(result.Items), 5,
				"At least 5 original tasks should be returned (none deleted by injection)")

			// 3. Verify table still exists (DROP TABLE didn't execute)
			verifyParams := domain.ListTasksParams{
				ListID: &listID,
				Limit:  100,
			}
			verifyResult, verifyErr := store.FindItems(ctx, verifyParams)
			require.NoError(t, verifyErr,
				"Table should still exist (DROP TABLE was blocked)")
			assert.GreaterOrEqual(t, len(verifyResult.Items), 5,
				"All data intact (table not dropped)")

			// 4. Verify task status unchanged (UPDATE didn't execute)
			// Count how many original "Test Task" items remain with TODO status
			originalTaskCount := 0
			for _, item := range verifyResult.Items {
				if item.Title == "Test Task" {
					assert.Equal(t, domain.TaskStatusTodo, item.Status,
						"Original task status should still be TODO (UPDATE was blocked)")
					originalTaskCount++
				}
			}
			assert.Equal(t, 5, originalTaskCount,
				"All 5 original tasks should still exist")

			// 5. Verify original task data unchanged
			// (Post-injection tasks are proof the table is not corrupted)
			for _, item := range verifyResult.Items {
				if item.Title == "Test Task" {
					// Verify original tasks have expected priority
					assert.NotNil(t, item.Priority, "Original task should have priority")
					if item.Priority != nil {
						assert.Equal(t, domain.TaskPriorityMedium, *item.Priority,
							"Original task priority unchanged")
					}
				}
			}

			// 6. Verify can insert new tasks (table not dropped/corrupted)
			priorityLow := domain.TaskPriorityLow
			newTaskUUID, err := uuid.NewV7()
			require.NoError(t, err, "failed to generate task UUID")
			newTask := domain.TodoItem{
				ID:       newTaskUUID.String(),
				Title:    "Post-injection task",
				Status:   domain.TaskStatusTodo,
				Priority: &priorityLow,
			}
			createErr := store.CreateItem(ctx, listID, &newTask)
			require.NoError(t, createErr,
				"Should create new tasks after injection attempt")

			t.Logf("✅ SQL INJECTION BLOCKED BY PARAMETERIZED QUERIES")
			t.Logf("   Malicious Input: %q", tc.orderBy)
			t.Logf("   Service Layer:   params.OrderBy = userInput  ← NO VALIDATION")
			t.Logf("   Storage Layer:   Passed malicious string as parameter $9")
			t.Logf("   SQL Layer:       Parameterized query ($9::text) treated it as STRING LITERAL")
			t.Logf("   PostgreSQL:      Never executed malicious SQL")
			t.Logf("   Result:          ✓ Query executed safely")
			t.Logf("                    ✓ %d tasks returned", len(result.Items))
			t.Logf("                    ✓ Table intact")
			t.Logf("                    ✓ No data corruption")
			t.Logf("\n%s", tc.flowExplanation)
		})
	}

	// Final comprehensive integrity check
	t.Run("comprehensive_integrity_check", func(t *testing.T) {
		// After all injection attempts, verify complete system integrity

		params := domain.ListTasksParams{
			ListID: &listID,
			Limit:  100,
		}
		result, err := store.FindItems(ctx, params)
		require.NoError(t, err)

		// Should have 5 original + 3 post-injection tasks
		assert.GreaterOrEqual(t, len(result.Items), 5,
			"All original tasks should exist after all injection attempts")

		// Verify different ORDER BY options work (table structure intact)
		for _, validOrder := range []string{"due_time", "priority", "created_at", "updated_at"} {
			orderParams := domain.ListTasksParams{
				ListID:  &listID,
				OrderBy: validOrder,
				Limit:   10,
			}
			resp, err := store.FindItems(ctx, orderParams)
			require.NoError(t, err,
				"Should support valid order_by: %s", validOrder)
			assert.NotEmpty(t, resp.Items,
				"Should return tasks with order_by: %s", validOrder)
		}

		t.Logf("✅ COMPREHENSIVE SYSTEM INTEGRITY CHECK PASSED")
		t.Logf("   - All tasks exist after multiple SQL injection attempts")
		t.Logf("   - Table structure completely unchanged")
		t.Logf("   - All ORDER BY options functional")
		t.Logf("   - System fully operational")
		t.Logf("   - Parameterized queries prevented ALL injection attempts")
	})
}

/*
TestSQLInjectionResistance_Explanation:

WHY PARAMETERIZED QUERIES PREVENT SQL INJECTION
(Even with Direct Assignment: params.OrderBy = req.OrderBy)

THE SCENARIO:
Service layer (mono.go:363): params.OrderBy = req.OrderBy  // NO validation!
SQL query (todo_items.sql:87): CASE WHEN $9::text = 'due_time' ...

WHAT HAPPENS:
1. Malicious input: "id; DROP TABLE todo_items--"
2. Service: params.OrderBy = "id; DROP TABLE todo_items--" (direct assignment)
3. Storage: db.ExecContext(ctx, sql, ..., params.OrderBy)
4. PostgreSQL receives:
   - SQL string (constant): "... CASE WHEN $9::text = ..."
   - Parameter $9: "id; DROP TABLE todo_items--" (AS DATA)
5. PostgreSQL executes: CASE WHEN 'id; DROP TABLE todo_items--' = 'due_time'
   The malicious string is a STRING LITERAL, not executable SQL
6. Result: Safe execution, tables intact

KEY POINTS:
✅ $9 is bound as DATA, not CODE (protocol-level protection)
✅ No string concatenation = no injection possible
✅ Direct assignment is SAFE with parameterized queries
✅ Validation helps UX, but security comes from parameterized queries
*/
